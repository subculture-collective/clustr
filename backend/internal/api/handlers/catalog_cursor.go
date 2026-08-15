package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"sort"
	"strings"
)

type catalogCursor struct {
	Revision    int64  `json:"r"`
	Catalog     int64  `json:"c"`
	Fingerprint string `json:"f"`
	Offset      int    `json:"o"`
}

type signedCatalogCursor struct {
	Payload catalogCursor `json:"p"`
	MAC     string        `json:"m"`
}

func catalogCursorKey() []byte {
	if configured := os.Getenv("CATALOG_CURSOR_SECRET"); configured != "" {
		return []byte(configured)
	}
	// Cursors contain only public snapshot offsets. This versioned fallback
	// keeps development deterministic; production config validation requires a
	// deployment-specific secret before Catalog is enabled.
	return []byte("clustr-public-catalog-cursor-v2-development")
}

func catalogFingerprint(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		switch key {
		case "cursor", "limit", "revision", "revision_id":
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	canonical := url.Values{}
	for _, key := range keys {
		items := append([]string(nil), values[key]...)
		sort.Strings(items)
		for _, item := range items {
			canonical.Add(key, strings.TrimSpace(item))
		}
	}
	sum := sha256.Sum256([]byte(canonical.Encode()))
	return hex.EncodeToString(sum[:])
}

func encodeCatalogCursor(cursor catalogCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, catalogCursorKey())
	_, _ = mac.Write(payload)
	wrapper := signedCatalogCursor{Payload: cursor, MAC: hex.EncodeToString(mac.Sum(nil))}
	body, err := json.Marshal(wrapper)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func decodeCatalogCursor(raw string, revision, catalog int64, values url.Values) (catalogCursor, error) {
	if raw == "" {
		return catalogCursor{Revision: revision, Catalog: catalog, Fingerprint: catalogFingerprint(values)}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return catalogCursor{}, errors.New("invalid catalog cursor")
	}
	var wrapper signedCatalogCursor
	if err := json.Unmarshal(body, &wrapper); err != nil || wrapper.Payload.Offset < 0 {
		return catalogCursor{}, errors.New("invalid catalog cursor")
	}
	payload, _ := json.Marshal(wrapper.Payload)
	provided, err := hex.DecodeString(wrapper.MAC)
	if err != nil {
		return catalogCursor{}, errors.New("invalid catalog cursor")
	}
	mac := hmac.New(sha256.New, catalogCursorKey())
	_, _ = mac.Write(payload)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return catalogCursor{}, errors.New("invalid catalog cursor")
	}
	fingerprint := catalogFingerprint(values)
	if wrapper.Payload.Revision != revision || wrapper.Payload.Catalog != catalog || wrapper.Payload.Fingerprint != fingerprint {
		return catalogCursor{}, errors.New("catalog cursor belongs to a different snapshot or query")
	}
	return wrapper.Payload, nil
}
