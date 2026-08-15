package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
)

func newRevisionHandlerMock(t *testing.T) (*RevisionHandler, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewRevisionHandler(database), mock
}

func expectPinnedRevision(mock sqlmock.Sqlmock, revision, catalog int64) {
	mock.ExpectQuery("SELECT status='published' FROM graph_revisions").WithArgs(revision).
		WillReturnRows(sqlmock.NewRows([]string{"published"}).AddRow(true))
	mock.ExpectQuery("SELECT spatial_catalog_id FROM graph_revisions").WithArgs(revision).
		WillReturnRows(sqlmock.NewRows([]string{"spatial_catalog_id"}).AddRow(catalog))
}

func TestTelemetryReturnsExactPinnedCatalogMeasurements(t *testing.T) {
	handler, mock := newRevisionHandlerMock(t)
	expectPinnedRevision(mock, 41, 73)
	mock.ExpectQuery("SELECT entity_count,link_count,subreddit_count").WithArgs(int64(73), int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"entities", "links", "subreddits", "users", "posts", "comments", "communities"}).
			AddRow(1000, 900, 10, 20, 300, 670, 4))
	mock.ExpectQuery("FROM spatial_catalog_entities WHERE catalog_id=\\$1 AND type='subreddit'").WithArgs(int64(73), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "label", "subscribers", "activity", "users"}).
			AddRow("subreddit_one", "One", 500, 40, 12))
	mock.ExpectQuery("FROM spatial_catalog_entities WHERE catalog_id=\\$1 AND type='user'").WithArgs(int64(73), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "label", "posts", "comments", "activity", "communities"}).
			AddRow("user_one", "Reader", 2, 8, 10, 3))

	recorder := httptest.NewRecorder()
	handler.Telemetry(recorder, httptest.NewRequest(http.MethodGet, "/api/graph/telemetry?revision=41&top_limit=2", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("telemetry status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var payload telemetryPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RevisionID != 41 || payload.SpatialCatalog != 73 || payload.Totals.Entities != 1000 || payload.Totals.ByType["comment"] != 670 {
		t.Fatalf("unexpected telemetry payload: %#v", payload)
	}
	if len(payload.TopSubreddits) != 1 || len(payload.TopUsers) != 1 {
		t.Fatalf("unexpected rankings: %#v", payload)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommunitiesPaginatesInStablePublishedOrder(t *testing.T) {
	handler, mock := newRevisionHandlerMock(t)
	expectPinnedRevision(mock, 41, 73)
	mock.ExpectQuery("FROM graph_revision_communities WHERE revision_id=\\$1 AND level=\\$2").
		WithArgs(int64(41), 0, 0, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "label", "size", "x", "y", "z"}).
			AddRow("c:alpha", "Alpha", 100, 1, 2, 3).
			AddRow("c:beta", "Beta", 90, 4, 5, 6))

	recorder := httptest.NewRecorder()
	handler.Communities(recorder, httptest.NewRequest(http.MethodGet, "/api/graph/communities?revision=41&level=0&limit=1", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("communities status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var payload publishedCommunitiesPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Communities) != 1 || payload.Communities[0].ID != "c:alpha" || payload.NextCursor == "" {
		t.Fatalf("unexpected communities payload: %#v", payload)
	}
	token, err := decodeCommunityCatalogCursor(payload.NextCursor)
	if err != nil || token != (communityCatalogCursor{Revision: 41, Level: 0, Offset: 1}) {
		t.Fatalf("cursor = %#v, err=%v", token, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNodeDetailsResolvesPublishedCommunityIdentity(t *testing.T) {
	handler, mock := newRevisionHandlerMock(t)
	expectPinnedRevision(mock, 41, 73)
	mock.ExpectQuery("SELECT id,label,value,type,x,y,z FROM spatial_catalog_entities").WithArgs(int64(73), "c:alpha").
		WillReturnRows(sqlmock.NewRows([]string{"id", "label", "value", "type", "x", "y", "z"}))
	mock.ExpectQuery("SELECT community_id,label,size,'community',x,y,z").WithArgs(int64(41), "c:alpha").
		WillReturnRows(sqlmock.NewRows([]string{"id", "label", "size", "type", "x", "y", "z"}).
			AddRow("c:alpha", "Alpha", 100, "community", 1, 2, 3))
	mock.ExpectQuery("WITH adjacent AS").WithArgs(int64(41), "c:alpha", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "label", "value", "type", "degree"}).
			AddRow("c:beta", "Beta", "90", "community", 1))

	recorder := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/nodes/c:alpha?revision=41", nil), map[string]string{"id": "c:alpha"})
	handler.NodeDetails(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("node details status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var payload NodeDetailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RevisionID != 41 || payload.CatalogID != 73 || payload.Type != "community" || payload.Name != "Alpha" || len(payload.Neighbors) != 1 {
		t.Fatalf("unexpected node details payload: %#v", payload)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
