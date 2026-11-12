package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/subculture-collective/clustr/backend/internal/admin"
	"github.com/subculture-collective/clustr/backend/internal/db"
)

type AdminHandler struct{ q *db.Queries }

func NewAdminHandler(q *db.Queries) *AdminHandler { return &AdminHandler{q: q} }

type ServiceState struct {
	CrawlerEnabled bool `json:"crawler_enabled"`
	PrecalcEnabled bool `json:"precalc_enabled"`
}

// GetServices returns current service flags.
func (h *AdminHandler) GetServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	crawler, _ := admin.GetBool(ctx, h.q, "crawler_enabled", true)
	precalc, _ := admin.GetBool(ctx, h.q, "precalc_enabled", true)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ServiceState{CrawlerEnabled: crawler, PrecalcEnabled: precalc})
}

type updateReq struct {
	CrawlerEnabled *bool `json:"crawler_enabled"`
	PrecalcEnabled *bool `json:"precalc_enabled"`
}

// UpdateServices sets service flags (partial updates allowed).
func (h *AdminHandler) UpdateServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.CrawlerEnabled != nil {
		_ = admin.Set(ctx, h.q, "crawler_enabled", map[bool]string{true: "true", false: "false"}[*req.CrawlerEnabled])
	}
	if req.PrecalcEnabled != nil {
		_ = admin.Set(ctx, h.q, "precalc_enabled", map[bool]string{true: "true", false: "false"}[*req.PrecalcEnabled])
	}
	h.GetServices(w, r)
}

// Note: Precalc control is intentionally kept for backward compatibility but
// the actual precalc runs in a separate service (cmd/precalculate). The API
// no longer exposes a run-now endpoint.
