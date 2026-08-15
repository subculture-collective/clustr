package handlers

import (
	"net/url"
	"testing"
)

func TestCatalogCursorIsBoundToSnapshotQueryFiltersAndSort(t *testing.T) {
	filters := url.Values{"q": {"woodworking"}, "type": {"subreddit"}, "sort": {"distinctiveness"}}
	token, err := encodeCatalogCursor(catalogCursor{Revision: 17, Catalog: 9, Fingerprint: catalogFingerprint(filters), Offset: 50})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCatalogCursor(token, 17, 9, filters)
	if err != nil {
		t.Fatalf("valid cursor rejected: %v", err)
	}
	if decoded.Offset != 50 {
		t.Fatalf("offset = %d, want 50", decoded.Offset)
	}

	changed := url.Values{"q": {"metalworking"}, "type": {"subreddit"}, "sort": {"distinctiveness"}}
	if _, err := decodeCatalogCursor(token, 17, 9, changed); err == nil {
		t.Fatal("cursor must be rejected after query changes")
	}
	if _, err := decodeCatalogCursor(token, 18, 9, filters); err == nil {
		t.Fatal("cursor must be rejected after revision changes")
	}
	if _, err := decodeCatalogCursor(token+"x", 17, 9, filters); err == nil {
		t.Fatal("tampered cursor must be rejected")
	}
}
