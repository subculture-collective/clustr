package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/config"
	"github.com/subculture-collective/clustr/backend/internal/db"
	"github.com/subculture-collective/clustr/backend/internal/logger"
	"github.com/subculture-collective/clustr/backend/internal/metrics"
	"github.com/subculture-collective/clustr/backend/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// ExportDataReader abstracts graph data access for export.
type ExportDataReader interface {
	GetPrecalculatedGraphDataCappedAll(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedAllParams) ([]db.GetPrecalculatedGraphDataCappedAllRow, error)
	GetPrecalculatedGraphDataCappedFiltered(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedFilteredParams) ([]db.GetPrecalculatedGraphDataCappedFilteredRow, error)
}

// ExportGraph handles GET /api/export?format=json|csv for exporting graph data.
func ExportGraph(q ExportDataReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "handlers.ExportGraph")
		defer span.End()

		// Parse format parameter (json or csv, default json)
		format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "" {
			format = "json"
		}
		if format != "json" && format != "csv" {
			http.Error(w, `{"error":"format must be json or csv"}`, http.StatusBadRequest)
			return
		}

		// Parse caps (default 10000 nodes, 25000 links)
		// Limits are enforced to be within int32 range for database compatibility
		maxNodes := parseIntDefault(r.URL.Query().Get("max_nodes"), 10000)
		maxLinks := parseIntDefault(r.URL.Query().Get("max_links"), 25000)

		// Cap at reasonable limits to prevent excessive exports (within int32 range)
		if maxNodes > 50000 {
			maxNodes = 50000
		}
		if maxLinks > 100000 {
			maxLinks = 100000
		}

		// Parse type filter
		_, allowedList, typeKey, allowAll := parseTypes(r.URL.Query().Get("types"))

		span.SetAttributes(
			attribute.String("format", format),
			attribute.Int("max_nodes", maxNodes),
			attribute.Int("max_links", maxLinks),
			attribute.String("type_filter", typeKey),
		)

		// Set timeout
		cfg := config.Load()
		timeout := cfg.GraphQueryTimeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Fetch data
		var rows []exportRow

		if allowAll {
			// Safe conversion: maxNodes and maxLinks are capped at 50k and 100k (well within int32 max)
			allRows, err := q.GetPrecalculatedGraphDataCappedAll(ctx, db.GetPrecalculatedGraphDataCappedAllParams{
				Limit:   int32(maxNodes),
				Limit_2: int32(maxLinks),
			})
			if err != nil {
				logger.ErrorContext(ctx, "Failed to fetch export data", "error", err)
				http.Error(w, `{"error":"Failed to fetch export data"}`, http.StatusInternalServerError)
				return
			}
			rows = convertToExportRows(allRows)
		} else {
			if len(allowedList) == 0 {
				// Empty filter - return empty result
				rows = []exportRow{}
			} else {
				// Safe conversion: maxNodes and maxLinks are capped at 50k and 100k (well within int32 max)
				filteredRows, err := q.GetPrecalculatedGraphDataCappedFiltered(ctx, db.GetPrecalculatedGraphDataCappedFilteredParams{
					Column1: allowedList,
					Limit:   int32(maxNodes),
					Limit_2: int32(maxLinks),
				})
				if err != nil {
					logger.ErrorContext(ctx, "Failed to fetch filtered export data", "error", err)
					http.Error(w, `{"error":"Failed to fetch export data"}`, http.StatusInternalServerError)
					return
				}
				rows = convertToExportRowsFiltered(filteredRows)
			}
		}

		// Track metrics
		metrics.APIRequestsTotal.WithLabelValues("/api/export", "GET", "200").Inc()
		span.SetAttributes(attribute.Int("rows_count", len(rows)))

		// Export in requested format
		if format == "csv" {
			exportCSV(w, rows)
		} else {
			exportJSON(w, rows)
		}
	}
}

type exportRow struct {
	DataType string
	ID       string
	Name     string
	Val      string
	Type     string
	Source   string
	Target   string
}

func convertToExportRows(rows []db.GetPrecalculatedGraphDataCappedAllRow) []exportRow {
	result := make([]exportRow, len(rows))
	for i, r := range rows {
		result[i] = exportRow{
			DataType: r.DataType,
			ID:       r.ID,
			Name:     r.Name,
			Val:      r.Val,
			Type:     r.Type.String,
			Source:   toString(r.Source),
			Target:   toString(r.Target),
		}
	}
	return result
}

func convertToExportRowsFiltered(rows []db.GetPrecalculatedGraphDataCappedFilteredRow) []exportRow {
	result := make([]exportRow, len(rows))
	for i, r := range rows {
		result[i] = exportRow{
			DataType: r.DataType,
			ID:       r.ID,
			Name:     r.Name,
			Val:      r.Val,
			Type:     r.Type.String,
			Source:   toString(r.Source),
			Target:   toString(r.Target),
		}
	}
	return result
}

func exportJSON(w http.ResponseWriter, rows []exportRow) {
	// Separate nodes and links for cleaner JSON structure
	var nodes []map[string]interface{}
	var links []map[string]interface{}

	for _, row := range rows {
		if strings.ToLower(row.DataType) == "node" {
			node := map[string]interface{}{
				"id":   row.ID,
				"name": row.Name,
				"val":  row.Val,
			}
			if row.Type != "" {
				node["type"] = row.Type
			}
			nodes = append(nodes, node)
		} else if strings.ToLower(row.DataType) == "link" {
			link := map[string]interface{}{
				"source": row.Source,
				"target": row.Target,
			}
			links = append(links, link)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=graph_export.json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"links": links,
	}); err != nil {
		logger.Error("Failed to encode JSON in exportJSON", "error", err)
	}
}

func exportCSV(w http.ResponseWriter, rows []exportRow) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=graph_export.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"data_type", "id", "name", "val", "type", "source", "target"}); err != nil {
		logger.Error("Failed to write CSV header", "error", err)
		return
	}

	// Write rows
	for _, row := range rows {
		record := []string{
			row.DataType,
			row.ID,
			row.Name,
			row.Val,
			row.Type,
			row.Source,
			row.Target,
		}
		if err := writer.Write(record); err != nil {
			logger.Error("Failed to write CSV row", "error", err)
			break
		}
	}
}
