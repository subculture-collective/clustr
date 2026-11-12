package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sqlc-dev/pqtype"
	"github.com/subculture-collective/clustr/backend/internal/admin"
	"github.com/subculture-collective/clustr/backend/internal/db"
	"github.com/subculture-collective/clustr/backend/internal/utils"
)

type AdminSettingsHandler struct {
	q *db.Queries
}

func NewAdminSettingsHandler(q *db.Queries) *AdminSettingsHandler {
	return &AdminSettingsHandler{q: q}
}

// SettingsResponse represents all configurable settings
type SettingsResponse struct {
	CrawlerEnabled     bool    `json:"crawler_enabled"`
	PrecalcEnabled     bool    `json:"precalc_enabled"`
	DetailedGraph      bool    `json:"detailed_graph"`
	CrawlerRPS         float64 `json:"crawler_rps"`
	RateLimitGlobal    float64 `json:"rate_limit_global"`
	RateLimitPerIP     float64 `json:"rate_limit_per_ip"`
	LayoutMaxNodes     int     `json:"layout_max_nodes"`
	LayoutIterations   int     `json:"layout_iterations"`
	PostsPerSubInGraph int     `json:"posts_per_sub_in_graph"`
	CommentsPerPost    int     `json:"comments_per_post_in_graph"`
	MaxAuthorLinks     int     `json:"max_author_content_links"`
	MaxPostsPerSub     int     `json:"max_posts_per_sub"`
}

// GetSettings returns all configurable settings
func (h *AdminSettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get service-specific settings from database
	crawlerEnabled, _ := admin.GetBool(ctx, h.q, "crawler_enabled", true)
	precalcEnabled, _ := admin.GetBool(ctx, h.q, "precalc_enabled", true)

	// Get other settings that might be stored in the database
	detailedGraph, _ := admin.GetBool(ctx, h.q, "detailed_graph", false)

	// Get numeric settings
	crawlerRPS := getFloatSetting(ctx, h.q, "crawler_rps", 1.66)
	rateLimitGlobal := getFloatSetting(ctx, h.q, "rate_limit_global", 100.0)
	rateLimitPerIP := getFloatSetting(ctx, h.q, "rate_limit_per_ip", 10.0)
	layoutMaxNodes := getIntSetting(ctx, h.q, "layout_max_nodes", 5000)
	layoutIterations := getIntSetting(ctx, h.q, "layout_iterations", 400)
	postsPerSub := getIntSetting(ctx, h.q, "posts_per_sub_in_graph", 10)
	commentsPerPost := getIntSetting(ctx, h.q, "comments_per_post_in_graph", 50)
	maxAuthorLinks := getIntSetting(ctx, h.q, "max_author_content_links", 3)
	maxPostsPerSub := getIntSetting(ctx, h.q, "max_posts_per_sub", 25)

	response := SettingsResponse{
		CrawlerEnabled:     crawlerEnabled,
		PrecalcEnabled:     precalcEnabled,
		DetailedGraph:      detailedGraph,
		CrawlerRPS:         crawlerRPS,
		RateLimitGlobal:    rateLimitGlobal,
		RateLimitPerIP:     rateLimitPerIP,
		LayoutMaxNodes:     layoutMaxNodes,
		LayoutIterations:   layoutIterations,
		PostsPerSubInGraph: postsPerSub,
		CommentsPerPost:    commentsPerPost,
		MaxAuthorLinks:     maxAuthorLinks,
		MaxPostsPerSub:     maxPostsPerSub,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateSettings updates configurable settings
func (h *AdminSettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	changes := make(map[string]interface{})

	// Update boolean settings
	if val, ok := req["crawler_enabled"].(bool); ok {
		if err := admin.Set(ctx, h.q, "crawler_enabled", boolToString(val)); err != nil {
			http.Error(w, "Failed to update crawler_enabled: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["crawler_enabled"] = val
	}
	if val, ok := req["precalc_enabled"].(bool); ok {
		if err := admin.Set(ctx, h.q, "precalc_enabled", boolToString(val)); err != nil {
			http.Error(w, "Failed to update precalc_enabled: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["precalc_enabled"] = val
	}
	if val, ok := req["detailed_graph"].(bool); ok {
		if err := admin.Set(ctx, h.q, "detailed_graph", boolToString(val)); err != nil {
			http.Error(w, "Failed to update detailed_graph: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["detailed_graph"] = val
	}

	// Update numeric settings
	if val, ok := req["crawler_rps"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "crawler_rps", floatToString(val)); err != nil {
			http.Error(w, "Failed to update crawler_rps: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["crawler_rps"] = val
	}
	if val, ok := req["rate_limit_global"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "rate_limit_global", floatToString(val)); err != nil {
			http.Error(w, "Failed to update rate_limit_global: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["rate_limit_global"] = val
	}
	if val, ok := req["rate_limit_per_ip"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "rate_limit_per_ip", floatToString(val)); err != nil {
			http.Error(w, "Failed to update rate_limit_per_ip: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["rate_limit_per_ip"] = val
	}
	if val, ok := req["layout_max_nodes"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "layout_max_nodes", intToString(int(val))); err != nil {
			http.Error(w, "Failed to update layout_max_nodes: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["layout_max_nodes"] = int(val)
	}
	if val, ok := req["layout_iterations"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "layout_iterations", intToString(int(val))); err != nil {
			http.Error(w, "Failed to update layout_iterations: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["layout_iterations"] = int(val)
	}
	if val, ok := req["posts_per_sub_in_graph"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "posts_per_sub_in_graph", intToString(int(val))); err != nil {
			http.Error(w, "Failed to update posts_per_sub_in_graph: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["posts_per_sub_in_graph"] = int(val)
	}
	if val, ok := req["comments_per_post_in_graph"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "comments_per_post_in_graph", intToString(int(val))); err != nil {
			http.Error(w, "Failed to update comments_per_post_in_graph: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["comments_per_post_in_graph"] = int(val)
	}
	if val, ok := req["max_author_content_links"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "max_author_content_links", intToString(int(val))); err != nil {
			http.Error(w, "Failed to update max_author_content_links: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["max_author_content_links"] = int(val)
	}
	if val, ok := req["max_posts_per_sub"].(float64); ok && val > 0 {
		if err := admin.Set(ctx, h.q, "max_posts_per_sub", intToString(int(val))); err != nil {
			http.Error(w, "Failed to update max_posts_per_sub: "+err.Error(), http.StatusInternalServerError)
			return
		}
		changes["max_posts_per_sub"] = int(val)
	}

	// Log the action if any changes were made
	if len(changes) > 0 {
		detailsJSON, _ := json.Marshal(changes)
		_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
			Action:       "update_settings",
			ResourceType: "settings",
			ResourceID:   sql.NullString{String: "system", Valid: true},
			UserID:       userID,
			Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
			IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
		})
	}

	// Return updated settings
	h.GetSettings(w, r)
}

// GetAuditLog returns the audit log entries
func (h *AdminSettingsHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit := 100
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := utils.Atoi32(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := utils.Atoi32(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	logs, err := h.q.ListAdminAuditLog(ctx, db.ListAdminAuditLogParams{
		Column1: int32(limit),
		Column2: int32(offset),
	})
	if err != nil {
		http.Error(w, "Failed to fetch audit log", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// Helper functions
func getFloatSetting(ctx context.Context, q *db.Queries, key string, def float64) float64 {
	val, _ := admin.Get(ctx, q, key)
	if val == "" {
		return def
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return def
}

func getIntSetting(ctx context.Context, q *db.Queries, key string, def int) int {
	val, _ := admin.Get(ctx, q, key)
	if val == "" {
		return def
	}
	if i, err := utils.Atoi32(val); err == nil {
		return i
	}
	return def
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func intToString(i int) string {
	return strconv.Itoa(i)
}
