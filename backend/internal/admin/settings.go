package admin

import (
	"context"
	"database/sql"
	"strings"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// Get returns the value for a key or empty string if not set.
func Get(ctx context.Context, q *db.Queries, key string) (string, error) {
	v, err := q.GetServiceSetting(ctx, key)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return v, nil
}

// Set sets the value for a key.
func Set(ctx context.Context, q *db.Queries, key, value string) error {
	return q.UpsertServiceSetting(ctx, db.UpsertServiceSettingParams{Key: key, Value: strings.TrimSpace(value)})
}

// GetBool reads a boolean with default if missing.
func GetBool(ctx context.Context, q *db.Queries, key string, def bool) (bool, error) {
	v, err := Get(ctx, q, key)
	if err != nil {
		return def, err
	}
	if v == "" {
		return def, nil
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return def, nil
	}
}
