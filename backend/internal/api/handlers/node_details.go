package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/onnwee/reddit-cluster-map/backend/internal/apierr"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
	"github.com/onnwee/reddit-cluster-map/backend/internal/metrics"
	"github.com/onnwee/reddit-cluster-map/backend/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// NodeDetailsReader abstracts node details queries for testability.
type NodeDetailsReader interface {
	GetNodeDetails(ctx context.Context, id string) (db.GetNodeDetailsRow, error)
	GetNodeNeighbors(ctx context.Context, arg db.GetNodeNeighborsParams) ([]db.GetNodeNeighborsRow, error)
	GetSubreddit(ctx context.Context, name string) (db.Subreddit, error)
	GetUser(ctx context.Context, username string) (db.User, error)
}

// NodeDetailResponse represents the detailed information about a node.
type NodeDetailResponse struct {
	RevisionID            int64             `json:"revision_id,omitempty"`
	CatalogID             int64             `json:"spatial_catalog_id,omitempty"`
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Val                   string            `json:"val"`
	Type                  string            `json:"type,omitempty"`
	PosX                  *float64          `json:"pos_x,omitempty"`
	PosY                  *float64          `json:"pos_y,omitempty"`
	PosZ                  *float64          `json:"pos_z,omitempty"`
	Degree                int               `json:"degree"`
	Neighbors             []NeighborInfo    `json:"neighbors"`
	Stats                 *NodeStats        `json:"stats,omitempty"`
	Title                 string            `json:"title,omitempty"`
	Body                  string            `json:"body,omitempty"`
	Description           string            `json:"description,omitempty"`
	SourcePermalink       string            `json:"source_permalink,omitempty"`
	CommunityID           string            `json:"community_id,omitempty"`
	MacroGroupID          string            `json:"macro_group_id,omitempty"`
	Distinctiveness       float64           `json:"distinctiveness,omitempty"`
	AffinityConfidence    float64           `json:"affinity_confidence,omitempty"`
	SignalComposition     json.RawMessage   `json:"signal_composition,omitempty"`
	RepresentativeMembers json.RawMessage   `json:"representative_members,omitempty"`
	Representative        bool              `json:"representative"`
	Sensitive             bool              `json:"sensitive"`
	Actions               map[string]string `json:"actions,omitempty"`
}

// NeighborInfo represents information about a neighboring node.
type NeighborInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Val    string `json:"val"`
	Type   string `json:"type,omitempty"`
	Degree int32  `json:"degree"`
}

// NodeStats represents type-specific statistics for a node.
type NodeStats struct {
	// Subreddit-specific fields
	Subscribers *int32  `json:"subscribers,omitempty"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`

	// User-specific fields (can be extended later)
	// Currently we just have basic user info from the users table
}

// GetNodeDetails handles GET /api/nodes/{id} for detailed node information.
func GetNodeDetails(q NodeDetailsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "handlers.GetNodeDetails")
		defer span.End()

		// Get node ID from URL path
		vars := mux.Vars(r)
		nodeID := vars["id"]
		if nodeID == "" {
			apierr.WriteErrorWithContext(w, r, apierr.SearchInvalidQuery("node id is required"))
			return
		}

		span.SetAttributes(attribute.String("node_id", nodeID))

		// Parse limit parameter for neighbors (default 10, max 100)
		neighborLimit := int32(10)
		if v := r.URL.Query().Get("neighbor_limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				if n > 100 {
					neighborLimit = 100
				} else {
					neighborLimit = int32(n)
				}
			}
		}

		// Fetch node details
		nodeDetails, err := q.GetNodeDetails(ctx, nodeID)
		if err != nil {
			if err == sql.ErrNoRows {
				apierr.WriteErrorWithContext(w, r, apierr.ResourceNotFound("node"))
				return
			}
			logger.ErrorContext(ctx, "Failed to get node details", "error", err, "node_id", nodeID)
			apierr.WriteErrorWithContext(w, r, apierr.SystemInternal("failed to fetch node details"))
			return
		}

		// Fetch neighbors
		neighbors, err := q.GetNodeNeighbors(ctx, db.GetNodeNeighborsParams{
			Source: nodeID,
			Limit:  neighborLimit,
		})
		if err != nil {
			logger.ErrorContext(ctx, "Failed to get node neighbors", "error", err, "node_id", nodeID)
			// Continue without neighbors rather than failing the whole request
			neighbors = []db.GetNodeNeighborsRow{}
		}

		// Build neighbor list
		neighborList := make([]NeighborInfo, len(neighbors))
		for i, n := range neighbors {
			valStr := ""
			if n.Val != nil {
				valStr = fmt.Sprint(n.Val)
			}
			neighborList[i] = NeighborInfo{
				ID:     n.ID,
				Name:   n.Name,
				Val:    valStr,
				Type:   n.Type.String,
				Degree: n.Degree,
			}
		}

		// Build response
		valStr := ""
		if nodeDetails.Val != nil {
			valStr = fmt.Sprint(nodeDetails.Val)
		}
		response := NodeDetailResponse{
			ID:        nodeDetails.ID,
			Name:      nodeDetails.Name,
			Val:       valStr,
			Type:      nodeDetails.Type.String,
			Degree:    len(neighbors),
			Neighbors: neighborList,
		}

		// Add position if available
		if nodeDetails.PosX.Valid {
			response.PosX = &nodeDetails.PosX.Float64
		}
		if nodeDetails.PosY.Valid {
			response.PosY = &nodeDetails.PosY.Float64
		}
		if nodeDetails.PosZ.Valid {
			response.PosZ = &nodeDetails.PosZ.Float64
		}

		// Fetch type-specific stats
		if nodeDetails.Type.Valid {
			stats, err := fetchNodeStats(ctx, q, nodeDetails.Type.String, nodeDetails.ID, nodeDetails.Name)
			if err != nil {
				logger.WarnContext(ctx, "Failed to fetch node stats", "error", err, "node_id", nodeID, "type", nodeDetails.Type.String)
				// Continue without stats
			} else if stats != nil {
				response.Stats = stats
			}
		}

		// Track metrics
		metrics.APIRequestsTotal.WithLabelValues("/api/nodes/{id}", "GET", "200").Inc()
		span.SetAttributes(
			attribute.Int("neighbor_count", len(neighbors)),
			attribute.String("node_type", nodeDetails.Type.String),
		)

		// Return response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.ErrorContext(ctx, "Failed to encode response", "error", err)
		}
	}
}

// fetchNodeStats fetches type-specific statistics for a node.
func fetchNodeStats(ctx context.Context, q NodeDetailsReader, nodeType, nodeID, nodeName string) (*NodeStats, error) {
	switch nodeType {
	case "subreddit":
		// Extract subreddit name from node ID (format: "subreddit_<id>")
		// But we need the actual name, which is in nodeName
		sub, err := q.GetSubreddit(ctx, nodeName)
		if err != nil {
			return nil, err
		}

		stats := &NodeStats{}
		if sub.Subscribers.Valid {
			stats.Subscribers = &sub.Subscribers.Int32
		}
		if sub.Title.Valid {
			stats.Title = &sub.Title.String
		}
		if sub.Description.Valid {
			stats.Description = &sub.Description.String
		}
		return stats, nil

	case "user":
		// For users, we could fetch activity stats, but for now just return nil
		// Can be extended later with GetUserTotalActivity, etc.
		return nil, nil

	default:
		return nil, nil
	}
}
