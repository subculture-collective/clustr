package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/subculture-collective/clustr/backend/internal/cache"
)

// CacheAdminHandler handles cache administration endpoints.
type CacheAdminHandler struct {
	cache cache.Cache
}

// NewCacheAdminHandler creates a new cache admin handler.
func NewCacheAdminHandler(c cache.Cache) *CacheAdminHandler {
	return &CacheAdminHandler{cache: c}
}

// InvalidateCache clears all entries from the cache.
// POST /api/admin/cache/invalidate
func (h *CacheAdminHandler) InvalidateCache(w http.ResponseWriter, r *http.Request) {
	h.cache.Clear()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Cache invalidated successfully",
	})
}

// GetCacheStats returns current cache statistics.
// GET /api/admin/cache/stats
func (h *CacheAdminHandler) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := h.cache.Stats()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hits":      stats.Hits,
		"misses":    stats.Misses,
		"keysAdded": stats.KeysAdded,
		"evictions": stats.Evictions,
		"sizeBytes": stats.Size,
		"items":     stats.Items,
	})
}
