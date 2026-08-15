package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
)

func TestCatalogEntityPageDoesNotLeakSensitiveText(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("SELECT r.id,r.spatial_catalog_id FROM graph_revision_current").
		WillReturnRows(sqlmock.NewRows([]string{"revision", "catalog"}).AddRow(7, 9))
	mock.ExpectQuery("SELECT EXISTS.*public_entity_suppressions").WithArgs("post_secret").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT e.label,COALESCE\\(d.title").WithArgs(int64(9), "post_secret", "post").
		WillReturnRows(sqlmock.NewRows([]string{"label", "title", "body", "description", "source", "source_sensitive", "admin_sensitive", "updated", "provenance"}).
			AddRow("private phrase in legacy label", "private title", "private body", "private description", "https://www.reddit.com/example", true, false, "2026-08-15", []byte(`{"source":"public"}`)))

	handler := NewCatalogPageHandler(database)
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/catalog/posts/post_secret", nil), map[string]string{"type": "posts", "id": "post_secret"})
	recorder := httptest.NewRecorder()
	handler.Entity(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, body)
	}
	for _, secret := range []string{"private phrase", "private title", "private body", "private description"} {
		if strings.Contains(body, secret) {
			t.Fatalf("sensitive text leaked in HTML: %q", secret)
		}
	}
	if !strings.Contains(body, "Sensitive post record") || !strings.Contains(body, `content="noindex,follow"`) {
		t.Fatalf("missing generic sensitive title or noindex: %s", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogEntityPageReturnsGoneForSuppression(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("SELECT r.id,r.spatial_catalog_id FROM graph_revision_current").
		WillReturnRows(sqlmock.NewRows([]string{"revision", "catalog"}).AddRow(7, 9))
	mock.ExpectQuery("SELECT EXISTS.*public_entity_suppressions").WithArgs("post_gone").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	handler := NewCatalogPageHandler(database)
	request := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/catalog/posts/post_gone", nil), map[string]string{"type": "posts", "id": "post_gone"})
	recorder := httptest.NewRecorder()
	handler.Entity(recorder, request)
	if recorder.Code != http.StatusGone || strings.Contains(recorder.Body.String(), "reason") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
