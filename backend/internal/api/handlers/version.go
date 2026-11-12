package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/subculture-collective/clustr/backend/internal/apierr"
	"github.com/subculture-collective/clustr/backend/internal/cache"
	"github.com/subculture-collective/clustr/backend/internal/db"
	"github.com/subculture-collective/clustr/backend/internal/logger"
)

// VersionReader interface for version-related queries
type VersionReader interface {
	GetCurrentGraphVersion(ctx context.Context) (db.GraphVersion, error)
	GetGraphDiffsSinceVersion(ctx context.Context, sinceVersion int64) ([]db.GetGraphDiffsSinceVersionRow, error)
	GetGraphVersion(ctx context.Context, id int64) (db.GraphVersion, error)
	ListGraphVersions(ctx context.Context, arg db.ListGraphVersionsParams) ([]db.GraphVersion, error)
}

// VersionHandler handles version-related HTTP requests
type VersionHandler struct {
	queries VersionReader
	cache   cache.Cache
}

// NewVersionHandler creates a new version handler
func NewVersionHandler(q VersionReader, c cache.Cache) *VersionHandler {
	return &VersionHandler{
		queries: q,
		cache:   c,
	}
}

// GraphVersionResponse represents the current graph version
type GraphVersionResponse struct {
	ID                int64  `json:"id"`
	CreatedAt         string `json:"created_at"`
	NodeCount         int32  `json:"node_count"`
	LinkCount         int32  `json:"link_count"`
	Status            string `json:"status"`
	PrecalcDurationMs *int32 `json:"precalc_duration_ms,omitempty"`
	IsFullRebuild     bool   `json:"is_full_rebuild"`
}

// GraphDiffEntry represents a single change in the graph
type GraphDiffEntry struct {
	VersionID  int64    `json:"version_id"`
	Action     string   `json:"action"` // add, remove, update
	EntityType string   `json:"entity_type"` // node, link
	EntityID   string   `json:"entity_id"`
	OldVal     *string  `json:"old_val,omitempty"`
	NewVal     *string  `json:"new_val,omitempty"`
	OldPosX    *float64 `json:"old_pos_x,omitempty"`
	OldPosY    *float64 `json:"old_pos_y,omitempty"`
	OldPosZ    *float64 `json:"old_pos_z,omitempty"`
	NewPosX    *float64 `json:"new_pos_x,omitempty"`
	NewPosY    *float64 `json:"new_pos_y,omitempty"`
	NewPosZ    *float64 `json:"new_pos_z,omitempty"`
}

// GraphDiffResponse represents changes since a version
type GraphDiffResponse struct {
	SinceVersion    int64            `json:"since_version"`
	CurrentVersion  int64            `json:"current_version"`
	Changes         []GraphDiffEntry `json:"changes"`
	TotalChanges    int              `json:"total_changes"`
	NodesAdded      int              `json:"nodes_added"`
	NodesRemoved    int              `json:"nodes_removed"`
	NodesUpdated    int              `json:"nodes_updated"`
	LinksAdded      int              `json:"links_added"`
	LinksRemoved    int              `json:"links_removed"`
}

// GetCurrentVersion returns the current graph version
// GET /api/graph/version
func (h *VersionHandler) GetCurrentVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Try cache first
	cacheKey := "graph:version:current"
	if cached, found := h.cache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		_, _ = w.Write(cached)
		return
	}
	
	// Fetch current version
	version, err := h.queries.GetCurrentGraphVersion(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			apierr.WriteError(w, apierr.ResourceNotFound("graph version"))
			return
		}
		logger.Error("Failed to get current graph version", "error", err)
		apierr.WriteError(w, apierr.SystemInternal("Failed to retrieve graph version"))
		return
	}
	
	// Build response
	response := GraphVersionResponse{
		ID:            version.ID,
		CreatedAt:     version.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		NodeCount:     version.NodeCount,
		LinkCount:     version.LinkCount,
		Status:        version.Status,
		IsFullRebuild: version.IsFullRebuild,
	}
	if version.PrecalcDurationMs.Valid {
		response.PrecalcDurationMs = &version.PrecalcDurationMs.Int32
	}
	
	// Serialize and cache
	data, err := json.Marshal(response)
	if err != nil {
		logger.Error("Failed to marshal version response", "error", err)
		apierr.WriteError(w, apierr.SystemInternal("Failed to serialize response"))
		return
	}
	
	// Cache for 60 seconds
	h.cache.Set(cacheKey, data, 60)
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	_, _ = w.Write(data)
}

// GetDiffSince returns changes since a specific version
// GET /api/graph/diff?since=N
func (h *VersionHandler) GetDiffSince(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Parse 'since' parameter
	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		apierr.WriteError(w, apierr.New(apierr.ErrValidationMissingField, "Missing 'since' parameter", http.StatusBadRequest))
		return
	}
	
	sinceVersion, err := strconv.ParseInt(sinceStr, 10, 64)
	if err != nil {
		apierr.WriteError(w, apierr.New(apierr.ErrValidationInvalidValue, "Invalid 'since' parameter: must be an integer", http.StatusBadRequest))
		return
	}
	
	if sinceVersion < 0 {
		apierr.WriteError(w, apierr.New(apierr.ErrValidationInvalidValue, "'since' parameter must be non-negative", http.StatusBadRequest))
		return
	}
	
	// Fetch current version first
	currentVersion, err := h.queries.GetCurrentGraphVersion(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			apierr.WriteError(w, apierr.ResourceNotFound("graph version"))
			return
		}
		logger.Error("Failed to get current graph version", "error", err)
		apierr.WriteError(w, apierr.SystemInternal("Failed to retrieve current version"))
		return
	}
	
	// Check if the requested version is valid
	if sinceVersion >= currentVersion.ID {
		apierr.WriteError(w, apierr.New(apierr.ErrValidationInvalidValue, "'since' version must be less than current version", http.StatusBadRequest))
		return
	}
	
	// Verify that the requested version still exists (has not been pruned by retention)
	if _, err := h.queries.GetGraphVersion(ctx, sinceVersion); err != nil {
		if err == sql.ErrNoRows {
			apierr.WriteError(w, apierr.New(apierr.ErrValidationInvalidValue, "'since' version is too old and no longer available; please refetch the full graph", http.StatusBadRequest))
			return
		}
		logger.Error("Failed to look up requested graph version", "error", err, "since", sinceVersion)
		apierr.WriteError(w, apierr.SystemInternal("Failed to validate requested version"))
		return
	}
	
	// Check cache (include current version in key to avoid stale data)
	cacheKey := fmt.Sprintf("graph:diff:since:%d:current:%d", sinceVersion, currentVersion.ID)
	if cached, found := h.cache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		_, _ = w.Write(cached)
		return
	}
	
	// Fetch diffs
	diffs, err := h.queries.GetGraphDiffsSinceVersion(ctx, sinceVersion)
	if err != nil {
		logger.Error("Failed to get graph diffs", "error", err, "since", sinceVersion)
		apierr.WriteError(w, apierr.SystemInternal("Failed to retrieve graph differences"))
		return
	}
	
	// Build response with statistics
	response := GraphDiffResponse{
		SinceVersion:   sinceVersion,
		CurrentVersion: currentVersion.ID,
		Changes:        make([]GraphDiffEntry, 0, len(diffs)),
		TotalChanges:   len(diffs),
	}
	
	for _, diff := range diffs {
		entry := GraphDiffEntry{
			VersionID:  diff.VersionID,
			Action:     diff.Action,
			EntityType: diff.EntityType,
			EntityID:   diff.EntityID,
		}
		
		if diff.OldVal.Valid {
			entry.OldVal = &diff.OldVal.String
		}
		if diff.NewVal.Valid {
			entry.NewVal = &diff.NewVal.String
		}
		if diff.OldPosX.Valid {
			entry.OldPosX = &diff.OldPosX.Float64
		}
		if diff.OldPosY.Valid {
			entry.OldPosY = &diff.OldPosY.Float64
		}
		if diff.OldPosZ.Valid {
			entry.OldPosZ = &diff.OldPosZ.Float64
		}
		if diff.NewPosX.Valid {
			entry.NewPosX = &diff.NewPosX.Float64
		}
		if diff.NewPosY.Valid {
			entry.NewPosY = &diff.NewPosY.Float64
		}
		if diff.NewPosZ.Valid {
			entry.NewPosZ = &diff.NewPosZ.Float64
		}
		
		// Count by action and entity type
		if diff.EntityType == "node" {
			switch diff.Action {
			case "add":
				response.NodesAdded++
			case "remove":
				response.NodesRemoved++
			case "update":
				response.NodesUpdated++
			}
		} else if diff.EntityType == "link" {
			switch diff.Action {
			case "add":
				response.LinksAdded++
			case "remove":
				response.LinksRemoved++
			}
		}
		
		response.Changes = append(response.Changes, entry)
	}
	
	// Serialize and cache
	data, err := json.Marshal(response)
	if err != nil {
		logger.Error("Failed to marshal diff response", "error", err)
		apierr.WriteError(w, apierr.SystemInternal("Failed to serialize response"))
		return
	}
	
	// Cache for 300 seconds (5 minutes)
	h.cache.Set(cacheKey, data, 300)
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	_, _ = w.Write(data)
}
