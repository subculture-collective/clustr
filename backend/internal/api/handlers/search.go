package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/subculture-collective/clustr/backend/internal/apierr"
	"github.com/subculture-collective/clustr/backend/internal/db"
	"github.com/subculture-collective/clustr/backend/internal/logger"
	"github.com/subculture-collective/clustr/backend/internal/metrics"
	"github.com/subculture-collective/clustr/backend/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// NodeSearcher abstracts node search for testability.
type NodeSearcher interface {
	SearchGraphNodes(ctx context.Context, arg db.SearchGraphNodesParams) ([]db.SearchGraphNodesRow, error)
}

// SearchNode handles GET /api/search?node=... for fuzzy node search.
func SearchNode(q NodeSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "handlers.SearchNode")
		defer span.End()

		// Get search query parameter
		query := strings.TrimSpace(r.URL.Query().Get("node"))
		if query == "" {
			apierr.WriteErrorWithContext(w, r, apierr.SearchInvalidQuery("node parameter is required"))
			return
		}

		// Parse limit parameter (default 50, max 500)
		limit := int32(50)
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				// Safely convert to int32 with bounds check
				if n > 500 {
					limit = 500
				} else {
					limit = int32(n)
				}
			}
		}

		span.SetAttributes(
			attribute.String("search_query", query),
			attribute.Int("limit", int(limit)),
		)

		// Execute search
		results, err := q.SearchGraphNodes(ctx, db.SearchGraphNodesParams{
			Column1: sql.NullString{String: query, Valid: true},
			Limit:   limit,
		})
		if err != nil {
			logger.ErrorContext(ctx, "Failed to search nodes", "error", err, "query", query)
			apierr.WriteErrorWithContext(w, r, apierr.SearchFailed(""))
			return
		}

		// Track search metrics
		metrics.APIRequestsTotal.WithLabelValues("/api/search", "GET", "200").Inc()
		span.SetAttributes(attribute.Int("results_count", len(results)))

		// Return results
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"query":   query,
			"count":   len(results),
			"results": results,
		}); err != nil {
			logger.ErrorContext(ctx, "Failed to encode response", "error", err)
		}
	}
}
