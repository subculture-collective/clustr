package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEntityCursorRoundTripBindsPinnedScope(t *testing.T) {
	want := entityCursor{
		Revision: 41,
		Catalog:  73,
		RootID:   "comment_parent",
		Expand:   "thread",
		Depth:    3,
		Offset:   250,
	}

	got, err := decodeEntityCursor(encodeEntityCursor(want))
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if got != want {
		t.Fatalf("decoded cursor = %#v, want %#v", got, want)
	}
}

func TestRevisionNodePublishesHierarchyAndTypedMetrics(t *testing.T) {
	node := revisionNode{
		ID: "comment_child", Name: "child", Type: "comment",
		ParentID: "comment_parent", AnchorID: "post_abc", AuthorID: "user_9",
		Metrics: json.RawMessage(`{"reply_count":4,"depth":1}`),
	}

	body, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(body)
	for _, want := range []string{`"parent_id":"comment_parent"`, `"anchor_id":"post_abc"`, `"author_id":"user_9"`, `"metrics":{"reply_count":4,"depth":1}`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("node JSON %s does not contain %s", encoded, want)
		}
	}
}

func TestEntityExpansionUsesBoundedDiscussionDefaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/graph/entity/post_abc?expand=discussion", nil)

	got, err := parseEntityExpansion(req)
	if err != nil {
		t.Fatalf("parse expansion: %v", err)
	}
	if got.Name != "discussion" || got.PageSize != 512 || got.MaxTotal != 2048 || got.Depth != 0 {
		t.Fatalf("expansion = %#v", got)
	}
}
