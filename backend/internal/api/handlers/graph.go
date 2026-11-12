package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/apierr"
	"github.com/onnwee/reddit-cluster-map/backend/internal/cache"
	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
	"github.com/onnwee/reddit-cluster-map/backend/internal/metrics"
	"github.com/onnwee/reddit-cluster-map/backend/internal/tracing"
	"github.com/onnwee/reddit-cluster-map/backend/internal/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Handler handles HTTP requests for the graph API.
type GraphDataReader interface {
	// Legacy aggregated JSON (users+subreddits only)
	GetGraphData(ctx context.Context) ([]json.RawMessage, error)
	// Precalculated graph tables (graph_nodes/graph_links)
	GetPrecalculatedGraphDataCappedAll(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedAllParams) ([]db.GetPrecalculatedGraphDataCappedAllRow, error)
	GetPrecalculatedGraphDataCappedFiltered(ctx context.Context, arg db.GetPrecalculatedGraphDataCappedFilteredParams) ([]db.GetPrecalculatedGraphDataCappedFilteredRow, error)
	GetPrecalculatedGraphDataNoPos(ctx context.Context) ([]db.GetPrecalculatedGraphDataNoPosRow, error)
	// Edge bundles
	GetEdgeBundles(ctx context.Context, weight int32) ([]db.GetEdgeBundlesRow, error)
	// Community aggregation
	GetCommunitySupernodesWithPositions(ctx context.Context) ([]db.GetCommunitySupernodesWithPositionsRow, error)
	GetCommunityLinks(ctx context.Context, limit int32) ([]db.GetCommunityLinksRow, error)
	// Spatial queries
	GetNodesInBoundingBox(ctx context.Context, arg db.GetNodesInBoundingBoxParams) ([]db.GetNodesInBoundingBoxRow, error)
	GetLinksForNodesInBoundingBox(ctx context.Context, arg db.GetLinksForNodesInBoundingBoxParams) ([]db.GetLinksForNodesInBoundingBoxRow, error)
	// Pagination
	GetPaginatedGraphNodes(ctx context.Context, arg db.GetPaginatedGraphNodesParams) ([]db.GetPaginatedGraphNodesRow, error)
	GetLinksForPaginatedNodes(ctx context.Context, arg db.GetLinksForPaginatedNodesParams) ([]db.GetLinksForPaginatedNodesRow, error)
}

func cacheKey(maxNodes, maxLinks int, typeKey string, withPositions bool) string {
	if typeKey == "" {
		typeKey = "all"
	}
	key := strconv.Itoa(maxNodes) + ":" + strconv.Itoa(maxLinks) + ":" + typeKey
	if withPositions {
		key += ":pos"
	}
	return key
}

type Handler struct {
	queries GraphDataReader
	cache   cache.Cache
}

// NewHandler creates a new graph handler.
func NewHandler(q GraphDataReader, c cache.Cache) *Handler {
	return &Handler{
		queries: q,
		cache:   c,
	}
}

type GraphNode struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Val  int      `json:"val"`
	Type string   `json:"type,omitempty"`
	X    *float64 `json:"x,omitempty"`
	Y    *float64 `json:"y,omitempty"`
	Z    *float64 `json:"z,omitempty"`
}

type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// PaginationInfo contains pagination metadata
type PaginationInfo struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	PageSize   int    `json:"page_size,omitempty"`
}

// PaginatedGraphResponse extends GraphResponse with pagination metadata
type PaginatedGraphResponse struct {
	Nodes      []GraphNode     `json:"nodes"`
	Links      []GraphLink     `json:"links"`
	Pagination *PaginationInfo `json:"pagination,omitempty"`
}

// cursorData represents the decoded cursor information
type cursorData struct {
	Weight int64
	ID     string
}

// encodeCursor creates a base64-encoded cursor from weight and ID
func encodeCursor(weight int64, id string) string {
	// Format: weight:id
	raw := fmt.Sprintf("%d:%s", weight, id)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor decodes a base64-encoded cursor into weight and ID
func decodeCursor(cursor string) (cursorData, error) {
	if cursor == "" {
		return cursorData{}, nil
	}
	
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return cursorData{}, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return cursorData{}, fmt.Errorf("invalid cursor format")
	}
	
	weight, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return cursorData{}, fmt.Errorf("invalid cursor weight: %w", err)
	}
	
	return cursorData{
		Weight: weight,
		ID:     parts[1],
	}, nil
}

// GetGraphData returns the graph data.
// It prefers the precalculated graph tables (graph_nodes/graph_links) when available,
// and falls back to the legacy aggregated JSON if none are present.
// Supports NDJSON streaming via Accept: application/x-ndjson header.
func (h *Handler) GetGraphData(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "handlers.GetGraphData")
	defer span.End()

	// Derive a bounded context to avoid very long queries
	cfg := config.Load()
	timeout := cfg.GraphQueryTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Optional caps and fallback control via query params
	maxNodes := parseIntDefault(r.URL.Query().Get("max_nodes"), 20000)
	maxLinks := parseIntDefault(r.URL.Query().Get("max_links"), 50000)
	fallback := r.URL.Query().Get("fallback")
	allowFallback := fallback == "" || fallback == "1" || strings.EqualFold(fallback, "true")
	allowedTypes, allowedList, typeKey, allowAll := parseTypes(r.URL.Query().Get("types"))
	withPos := func() bool {
		v := strings.TrimSpace(r.URL.Query().Get("with_positions"))
		return v == "1" || strings.EqualFold(v, "true")
	}()

	// Check if NDJSON streaming is requested
	acceptHeader := r.Header.Get("Accept")
	useNDJSON := strings.Contains(acceptHeader, "application/x-ndjson")
	// Check if pagination is requested
	cursorParam := r.URL.Query().Get("cursor")
	pageSizeParam := r.URL.Query().Get("page_size")
	if cursorParam != "" || pageSizeParam != "" {
		// Use pagination path
		h.getGraphDataPaginated(w, r, ctx, cursorParam, pageSizeParam, withPos, allowAll, allowedTypes)
		return
	}

	// Add attributes to span
	span.SetAttributes(
		attribute.Int("max_nodes", maxNodes),
		attribute.Int("max_links", maxLinks),
		attribute.Bool("with_positions", withPos),
		attribute.String("type_filter", typeKey),
		attribute.Bool("ndjson", useNDJSON),
	)

	if !allowAll && len(allowedTypes) == 0 {
		span.SetAttributes(attribute.String("result", "empty_filter"))
		writeCachedEmpty(w, h, maxNodes, maxLinks, typeKey, withPos)
		return
	}

	// Check cache first
	key := cacheKey(maxNodes, maxLinks, typeKey, withPos)
	if cachedData, found := h.cache.Get(key); found {
		metrics.APICacheHits.WithLabelValues("graph").Inc()
		span.SetAttributes(attribute.Bool("cache_hit", true))
		w.Header().Set("Content-Type", "application/json")
		w.Write(cachedData)
		return
	}
	metrics.APICacheMisses.WithLabelValues("graph").Inc()
	span.SetAttributes(attribute.Bool("cache_hit", false))

	// Try precalculated tables (capped) first
	rows, err := fetchPrecalcCapped(ctx, h.queries, maxNodes, maxLinks, allowAll, allowedList)
	if err != nil {
		// Check if this was a timeout/cancellation
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			logger.WarnContext(ctx, "Precalc query timed out", "timeout", timeout)
			span.RecordError(err)
			span.SetStatus(codes.Error, "query timeout")
			apierr.WriteErrorWithContext(w, r, apierr.GraphTimeout(""))
			return
		}
		if ctx.Err() == context.Canceled || err == context.Canceled {
			logger.WarnContext(ctx, "Precalc query was canceled")
			span.RecordError(err)
			span.SetStatus(codes.Error, "query canceled")
			apierr.WriteErrorWithContext(w, r, apierr.SystemTimeout("Request canceled"))
			return
		}
		logger.WarnContext(ctx, "Precalc capped query failed, falling back", "error", err)
		span.AddEvent("precalc_query_failed")
		// Continue to fallback
	}
	if len(rows) > 0 {
		// Build nodes/links then apply caps
		nodes := make(map[string]GraphNode, len(rows))
		links := make([]GraphLink, 0, len(rows))
		for _, row := range rows {
			switch strings.ToLower(row.DataType) {
			case "node":
				v := atoiSafe(row.Val)
				t := ""
				if (row.Type != sql.NullString{}) && row.Type.Valid {
					t = strings.ToLower(row.Type.String)
				}
				if !allowAll {
					if len(allowedTypes) == 0 {
						continue
					}
					if t != "" {
						if _, ok := allowedTypes[t]; !ok {
							continue
						}
					} else {
						continue
					}
				}
				gn := GraphNode{ID: row.ID, Name: row.Name, Val: v, Type: t}
				if withPos {
					if row.PosX.Valid {
						x := row.PosX.Float64
						gn.X = &x
					}
					if row.PosY.Valid {
						y := row.PosY.Float64
						gn.Y = &y
					}
					if row.PosZ.Valid {
						z := row.PosZ.Float64
						gn.Z = &z
					}
				}
				nodes[row.ID] = gn
			case "link":
				src := toString(row.Source)
				tgt := toString(row.Target)
				if src != "" && tgt != "" {
					links = append(links, GraphLink{Source: src, Target: tgt})
				}
			}
		}
		resp := capGraph(nodes, links, maxNodes, maxLinks)
		
		// Handle NDJSON streaming vs regular JSON
		if useNDJSON {
			writeNDJSONResponse(w, resp)
			// Note: NDJSON responses are not cached as they are streamed
			return
		}
		
		// Marshal once so we can both write and cache it
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
		// store in cache
		h.cache.Set(key, b, 0) // 0 means use default TTL
		return
	}

	// Fallback to legacy aggregated JSON (users+subreddits only)
	if !allowFallback {
		empty := GraphResponse{Nodes: []GraphNode{}, Links: []GraphLink{}}
		if useNDJSON {
			writeNDJSONResponse(w, empty)
		} else {
			w.Header().Set("Content-Type", "application/json")
			b, _ := json.Marshal(empty)
			_, _ = w.Write(b)
			// cache empty response too
			h.cache.Set(key, b, 0)
		}
		return
	}
	handleLegacyGraph(ctx, w, r, h, maxNodes, maxLinks, allowAll, allowedTypes, typeKey, withPos, useNDJSON)
}

func toString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		// generated type may use []uint8 for TEXT
		if b, ok := x.([]byte); ok {
			return string(b)
		}
		return ""
	}
}

// atoiSafe parses an int from text, returning 0 on error.
func atoiSafe(s string) int {
	if s == "" {
		return 0
	}
	if iv, err := utils.Atoi32(s); err == nil {
		return iv
	}
	return 0
}

// capGraph selects up to maxNodes by weight and filters links accordingly.
// Weight prefers higher Val and degree.
func capGraph(nodes map[string]GraphNode, links []GraphLink, maxNodes, maxLinks int) GraphResponse {
	if maxNodes <= 0 {
		maxNodes = 20000
	}
	if maxLinks <= 0 {
		maxLinks = 50000
	}
	// degree count
	deg := make(map[string]int, len(nodes))
	for _, l := range links {
		deg[l.Source]++
		deg[l.Target]++
	}
	// slice and sort by weight
	list := make([]GraphNode, 0, len(nodes))
	for _, n := range nodes {
		list = append(list, n)
	}
	// weight = max(Val, degree)
	sort.Slice(list, func(i, j int) bool {
		wi := list[i].Val
		if di := deg[list[i].ID]; di > wi {
			wi = di
		}
		wj := list[j].Val
		if dj := deg[list[j].ID]; dj > wj {
			wj = dj
		}
		if wi == wj {
			return list[i].ID < list[j].ID
		}
		return wi > wj
	})
	if len(list) > maxNodes {
		list = list[:maxNodes]
	}
	keep := make(map[string]struct{}, len(list))
	for _, n := range list {
		keep[n.ID] = struct{}{}
	}
	keptLinks := make([]GraphLink, 0, min(maxLinks, len(links)))
	for _, l := range links {
		if _, ok := keep[l.Source]; !ok {
			continue
		}
		if _, ok := keep[l.Target]; !ok {
			continue
		}
		keptLinks = append(keptLinks, l)
		if len(keptLinks) >= maxLinks {
			break
		}
	}
	return GraphResponse{Nodes: list, Links: keptLinks}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := utils.Atoi32(s); err == nil {
		return v
	}
	return def
}

func handleLegacyGraph(ctx context.Context, w http.ResponseWriter, r *http.Request, h *Handler, maxNodes, maxLinks int, allowAll bool, allowedTypes map[string]struct{}, typeKey string, withPos bool, useNDJSON bool) {
	cacheKeyStr := cacheKey(maxNodes, maxLinks, typeKey, withPos)
	data, err := h.queries.GetGraphData(ctx)
	if err != nil {
		apierr.WriteErrorWithContext(w, r, apierr.GraphQueryFailed("Failed to fetch graph data"))
		return
	}
	
	var response GraphResponse
	if len(data) == 1 {
		if err := json.Unmarshal(data[0], &response); err == nil {
			nodes := make(map[string]GraphNode, len(response.Nodes))
			for _, n := range response.Nodes {
				t := strings.ToLower(n.Type)
				if !allowAll {
					if len(allowedTypes) == 0 {
						continue
					}
					if t == "" {
						continue
					}
					if _, ok := allowedTypes[t]; !ok {
						continue
					}
				}
				n.Type = t
				nodes[n.ID] = n
			}
			response = capGraph(nodes, response.Links, maxNodes, maxLinks)
		} else {
			// Unknown legacy format; return empty
			response = GraphResponse{Nodes: []GraphNode{}, Links: []GraphLink{}}
		}
	} else {
		// No data or unexpected format; return empty
		response = GraphResponse{Nodes: []GraphNode{}, Links: []GraphLink{}}
	}
	
	// Write response based on format
	writeGraphResponse(w, response, useNDJSON, cacheKeyStr, h)
}

// writeGraphResponse writes a GraphResponse in either NDJSON or JSON format
func writeGraphResponse(w http.ResponseWriter, resp GraphResponse, useNDJSON bool, cacheKey string, h *Handler) {
	if useNDJSON {
		writeNDJSONResponse(w, resp)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(resp)
	_, _ = w.Write(b)
	if h != nil && cacheKey != "" {
		h.cache.Set(cacheKey, b, 0)
	}
}

// preRow is an internal union row type for capped precalc results including optional positions
type preRow struct {
	DataType string
	ID       string
	Name     string
	Val      string
	Type     sql.NullString
	PosX     sql.NullFloat64
	PosY     sql.NullFloat64
	PosZ     sql.NullFloat64
	Source   interface{}
	Target   interface{}
}

// fetchPrecalcCapped runs a DB-level capped selection of precalculated graph data.
func fetchPrecalcCapped(ctx context.Context, q GraphDataReader, maxNodes, maxLinks int, allowAll bool, allowedTypes []string) ([]preRow, error) {
	if maxNodes <= 0 {
		maxNodes = 20000
	}
	if maxLinks <= 0 {
		maxLinks = 50000
	}
	if allowAll {
		allRows, err := q.GetPrecalculatedGraphDataCappedAll(ctx, db.GetPrecalculatedGraphDataCappedAllParams{
			Limit:   int32(maxNodes),
			Limit_2: int32(maxLinks),
		})
		if err != nil {
			return nil, err
		}
		out := make([]preRow, len(allRows))
		for i, r := range allRows {
			out[i] = preRow{DataType: r.DataType, ID: r.ID, Name: r.Name, Val: r.Val, Type: r.Type, PosX: r.PosX, PosY: r.PosY, PosZ: r.PosZ, Source: r.Source, Target: r.Target}
		}
		return out, nil
	}
	arr := make([]string, len(allowedTypes))
	copy(arr, allowedTypes)
	filteredRows, err := q.GetPrecalculatedGraphDataCappedFiltered(ctx, db.GetPrecalculatedGraphDataCappedFilteredParams{
		Column1: arr,
		Limit:   int32(maxNodes),
		Limit_2: int32(maxLinks),
	})
	if err != nil {
		return nil, err
	}
	out := make([]preRow, len(filteredRows))
	for i, r := range filteredRows {
		out[i] = preRow{DataType: r.DataType, ID: r.ID, Name: r.Name, Val: r.Val, Type: r.Type, PosX: r.PosX, PosY: r.PosY, PosZ: r.PosZ, Source: r.Source, Target: r.Target}
	}
	return out, nil
}

func parseTypes(raw string) (map[string]struct{}, []string, string, bool) {
	if raw == "" {
		return nil, nil, "all", true
	}
	parts := strings.Split(raw, ",")
	allowed := make(map[string]struct{})
	for _, p := range parts {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" {
			continue
		}
		allowed[t] = struct{}{}
	}
	if len(allowed) == 0 {
		return map[string]struct{}{}, []string{}, "none:" + raw, false
	}
	list := make([]string, 0, len(allowed))
	for t := range allowed {
		list = append(list, t)
	}
	sort.Strings(list)
	return allowed, list, strings.Join(list, ","), false
}

func writeCachedEmpty(w http.ResponseWriter, h *Handler, maxNodes, maxLinks int, typeKey string, withPos bool) {
	w.Header().Set("Content-Type", "application/json")
	empty := GraphResponse{Nodes: []GraphNode{}, Links: []GraphLink{}}
	b, _ := json.Marshal(empty)
	_, _ = w.Write(b)
	h.cache.Set(cacheKey(maxNodes, maxLinks, typeKey, withPos), b, 0)
}

// NDJSONEnvelope wraps individual items in NDJSON stream
type NDJSONEnvelope struct {
	Type string      `json:"type"` // "node", "link", or "meta"
	Data interface{} `json:"data,omitempty"`
	// Meta fields (only when Type == "meta")
	TotalNodes *int `json:"totalNodes,omitempty"`
	TotalLinks *int `json:"totalLinks,omitempty"`
}

// writeNDJSONResponse streams graph data as NDJSON
func writeNDJSONResponse(w http.ResponseWriter, resp GraphResponse) {
	// Set headers for NDJSON streaming
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	
	// Get flusher for streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Fallback to regular JSON if streaming not supported
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	
	encoder := json.NewEncoder(w)
	
	// Stream nodes first
	for _, node := range resp.Nodes {
		envelope := NDJSONEnvelope{
			Type: "node",
			Data: node,
		}
		if err := encoder.Encode(envelope); err != nil {
			// Log error but continue - response already started
			logger.Warn("NDJSON encode error for node", "error", err)
			return
		}
	}
	// Flush after all nodes to ensure client can start rendering
	flusher.Flush()
	
	// Stream links
	for _, link := range resp.Links {
		envelope := NDJSONEnvelope{
			Type: "link",
			Data: link,
		}
		if err := encoder.Encode(envelope); err != nil {
			logger.Warn("NDJSON encode error for link", "error", err)
			return
		}
	}
	flusher.Flush()
	
	// Send metadata at the end
	totalNodes := len(resp.Nodes)
	totalLinks := len(resp.Links)
	metaEnvelope := NDJSONEnvelope{
		Type:       "meta",
		TotalNodes: &totalNodes,
		TotalLinks: &totalLinks,
	}
	if err := encoder.Encode(metaEnvelope); err != nil {
		logger.Warn("NDJSON encode error for meta", "error", err)
		return
	}
	flusher.Flush()
}

// EdgeBundle represents a bundled edge between two communities
type EdgeBundle struct {
	SourceCommunity int32    `json:"source_community"`
	TargetCommunity int32    `json:"target_community"`
	Weight          int32    `json:"weight"`
	AvgStrength     *float64 `json:"avg_strength,omitempty"`
	ControlPoint    *struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"control_point,omitempty"`
}

// EdgeBundlesResponse represents the response for edge bundles
type EdgeBundlesResponse struct {
	Bundles []EdgeBundle `json:"bundles"`
}

// GetEdgeBundles returns precomputed edge bundle metadata
// GET /api/graph/bundles?min_weight=1
func (h *Handler) GetEdgeBundles(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "handlers.GetEdgeBundles")
	defer span.End()

	cfg := config.Load()
	timeout := cfg.GraphQueryTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse min_weight parameter
	minWeight := parseIntDefault(r.URL.Query().Get("min_weight"), 1)
	if minWeight < 0 {
		minWeight = 0
	}
	// Clamp to MaxInt32 to prevent overflow when casting to int32
	if minWeight > math.MaxInt32 {
		minWeight = math.MaxInt32
	}

	// Check cache first
	cacheKeyStr := "bundles:" + strconv.Itoa(minWeight)
	if cachedData, found := h.cache.Get(cacheKeyStr); found {
		metrics.APICacheHits.WithLabelValues("bundles").Inc()
		w.Header().Set("Content-Type", "application/json")
		w.Write(cachedData)
		return
	}
	metrics.APICacheMisses.WithLabelValues("bundles").Inc()

	// Fetch bundles from database
	bundlesRows, err := h.queries.GetEdgeBundles(ctx, int32(minWeight))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			logger.WarnContext(ctx, "bundles query timed out", "timeout", timeout)
			apierr.WriteErrorWithContext(w, r, apierr.GraphTimeout("Bundles query timeout"))
			return
		}
		logger.ErrorContext(ctx, "failed to fetch edge bundles", "error", err)
		apierr.WriteErrorWithContext(w, r, apierr.GraphQueryFailed("Failed to fetch edge bundles"))
		return
	}

	// Build response
	bundles := make([]EdgeBundle, 0, len(bundlesRows))
	for _, row := range bundlesRows {
		bundle := EdgeBundle{
			SourceCommunity: row.SourceCommunityID,
			TargetCommunity: row.TargetCommunityID,
			Weight:          row.Weight,
		}

		if row.AvgStrength.Valid {
			strength := row.AvgStrength.Float64
			bundle.AvgStrength = &strength
		}

		if row.ControlX.Valid && row.ControlY.Valid && row.ControlZ.Valid {
			bundle.ControlPoint = &struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			}{
				X: row.ControlX.Float64,
				Y: row.ControlY.Float64,
				Z: row.ControlZ.Float64,
			}
		}

		bundles = append(bundles, bundle)
	}

	resp := EdgeBundlesResponse{Bundles: bundles}
	b, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)

	// Store in cache
	h.cache.Set(cacheKeyStr, b, 0)

	span.SetAttributes(
		attribute.Int("bundles_count", len(bundles)),
		attribute.Int("min_weight", minWeight),
	)
	span.SetStatus(codes.Ok, "bundles fetched successfully")
}

// GetGraphOverview returns a lightweight community-level overview of the graph.
// This is typically <1k nodes and provides a high-level view before drill-down.
// GET /api/graph/overview?max_nodes=100&max_links=500&with_positions=true
func (h *Handler) GetGraphOverview(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "handlers.GetGraphOverview")
	defer span.End()

	cfg := config.Load()
	timeout := cfg.GraphQueryTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse query parameters with conservative defaults for overview
	maxNodes := parseIntDefault(r.URL.Query().Get("max_nodes"), 100)
	maxLinks := parseIntDefault(r.URL.Query().Get("max_links"), 500)
	withPos := func() bool {
		v := strings.TrimSpace(r.URL.Query().Get("with_positions"))
		return v == "1" || strings.EqualFold(v, "true")
	}()

	span.SetAttributes(
		attribute.Int("max_nodes", maxNodes),
		attribute.Int("max_links", maxLinks),
		attribute.Bool("with_positions", withPos),
	)

	// Check cache first
	key := "overview:" + strconv.Itoa(maxNodes) + ":" + strconv.Itoa(maxLinks)
	if withPos {
		key += ":pos"
	}
	if cachedData, found := h.cache.Get(key); found {
		metrics.APICacheHits.WithLabelValues("graph_overview").Inc()
		span.SetAttributes(attribute.Bool("cache_hit", true))
		w.Header().Set("Content-Type", "application/json")
		w.Write(cachedData)
		return
	}
	metrics.APICacheMisses.WithLabelValues("graph_overview").Inc()
	span.SetAttributes(attribute.Bool("cache_hit", false))

	// Fetch community supernodes
	supernodesRows, err := h.queries.GetCommunitySupernodesWithPositions(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			logger.WarnContext(ctx, "overview query timed out", "timeout", timeout)
			span.RecordError(err)
			span.SetStatus(codes.Error, "query timeout")
			apierr.WriteErrorWithContext(w, r, apierr.GraphTimeout("Overview query timeout"))
			return
		}
		logger.ErrorContext(ctx, "failed to fetch community supernodes", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "query failed")
		apierr.WriteErrorWithContext(w, r, apierr.GraphQueryFailed("Failed to fetch overview"))
		return
	}

	// Fetch inter-community links
	linksRows, err := h.queries.GetCommunityLinks(ctx, int32(maxLinks))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			logger.WarnContext(ctx, "community links query timed out", "timeout", timeout)
			span.RecordError(err)
			span.SetStatus(codes.Error, "query timeout")
			apierr.WriteErrorWithContext(w, r, apierr.GraphTimeout("Overview query timeout"))
			return
		}
		logger.ErrorContext(ctx, "failed to fetch community links", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "query failed")
		apierr.WriteErrorWithContext(w, r, apierr.GraphQueryFailed("Failed to fetch community links"))
		return
	}

	// Build response - same format as /api/graph for frontend compatibility
	nodes := make([]GraphNode, 0, len(supernodesRows))
	for _, row := range supernodesRows {
		if len(nodes) >= maxNodes {
			break
		}
		v := atoiSafe(row.Val)
		gn := GraphNode{
			ID:   row.ID,
			Name: row.Name,
			Val:  v,
			Type: "community",
		}
		if withPos {
			// Always include positions when requested, even if they are zero
			// Convert interface{} to float64 (sqlc generates interface{} for COALESCE results)
			var x, y, z float64
			if posX, ok := row.PosX.(float64); ok {
				x = posX
			}
			if posY, ok := row.PosY.(float64); ok {
				y = posY
			}
			if posZ, ok := row.PosZ.(float64); ok {
				z = posZ
			}
			gn.X = &x
			gn.Y = &y
			gn.Z = &z
		}
		nodes = append(nodes, gn)
	}

	links := make([]GraphLink, 0, len(linksRows))
	for _, row := range linksRows {
		src := toString(row.Source)
		tgt := toString(row.Target)
		if src != "" && tgt != "" {
			links = append(links, GraphLink{Source: src, Target: tgt})
		}
	}

	resp := GraphResponse{Nodes: nodes, Links: links}
	b, err := json.Marshal(resp)
	if err != nil {
		logger.ErrorContext(ctx, "failed to marshal overview response", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "marshal failed")
		apierr.WriteErrorWithContext(w, r, apierr.SystemInternal("Failed to marshal response"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)

	// Store in cache
	h.cache.Set(key, b, 0)

	span.SetAttributes(
		attribute.Int("nodes_count", len(nodes)),
		attribute.Int("links_count", len(links)),
	)
	span.SetStatus(codes.Ok, "overview fetched successfully")
}

// GetGraphRegion returns nodes and links within a 3D bounding box.
// This enables spatial viewport queries for efficient rendering.
// GET /api/graph/region?x_min=&x_max=&y_min=&y_max=&z_min=&z_max=&max_nodes=10000&max_links=50000
func (h *Handler) GetGraphRegion(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "handlers.GetGraphRegion")
	defer span.End()

	cfg := config.Load()
	timeout := cfg.GraphQueryTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse bounding box parameters
	query := r.URL.Query()
	xMin, err := strconv.ParseFloat(query.Get("x_min"), 64)
	if err != nil {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid x_min parameter"))
		return
	}
	xMax, err := strconv.ParseFloat(query.Get("x_max"), 64)
	if err != nil {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid x_max parameter"))
		return
	}
	yMin, err := strconv.ParseFloat(query.Get("y_min"), 64)
	if err != nil {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid y_min parameter"))
		return
	}
	yMax, err := strconv.ParseFloat(query.Get("y_max"), 64)
	if err != nil {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid y_max parameter"))
		return
	}
	zMin, err := strconv.ParseFloat(query.Get("z_min"), 64)
	if err != nil {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid z_min parameter"))
		return
	}
	zMax, err := strconv.ParseFloat(query.Get("z_max"), 64)
	if err != nil {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid z_max parameter"))
		return
	}

	// Reject non-finite bounding box values (NaN, +Inf, -Inf)
	if math.IsNaN(xMin) || math.IsInf(xMin, 0) || math.IsNaN(xMax) || math.IsInf(xMax, 0) ||
		math.IsNaN(yMin) || math.IsInf(yMin, 0) || math.IsNaN(yMax) || math.IsInf(yMax, 0) ||
		math.IsNaN(zMin) || math.IsInf(zMin, 0) || math.IsNaN(zMax) || math.IsInf(zMax, 0) {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Bounding box values must be finite"))
		return
	}

	// Validate bounding box
	if xMin > xMax || yMin > yMax || zMin > zMax {
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid bounding box: min values must be <= max values"))
		return
	}

	// Parse additional parameters
	maxNodes := parseIntDefault(query.Get("max_nodes"), 10000)
	maxLinks := parseIntDefault(query.Get("max_links"), 50000)

	// Clamp to positive range and int32 range for database
	if maxNodes <= 0 {
		maxNodes = 1
	}
	if maxLinks <= 0 {
		maxLinks = 1
	}
	if maxNodes > math.MaxInt32 {
		maxNodes = math.MaxInt32
	}
	if maxLinks > math.MaxInt32 {
		maxLinks = math.MaxInt32
	}

	span.SetAttributes(
		attribute.Float64("x_min", xMin),
		attribute.Float64("x_max", xMax),
		attribute.Float64("y_min", yMin),
		attribute.Float64("y_max", yMax),
		attribute.Float64("z_min", zMin),
		attribute.Float64("z_max", zMax),
		attribute.Int("max_nodes", maxNodes),
		attribute.Int("max_links", maxLinks),
	)

	// Build cache key using a high-precision, lossless representation of the bounding box
	key := "region:" + strconv.FormatFloat(xMin, 'g', 17, 64) + ":" +
		strconv.FormatFloat(xMax, 'g', 17, 64) + ":" +
		strconv.FormatFloat(yMin, 'g', 17, 64) + ":" +
		strconv.FormatFloat(yMax, 'g', 17, 64) + ":" +
		strconv.FormatFloat(zMin, 'g', 17, 64) + ":" +
		strconv.FormatFloat(zMax, 'g', 17, 64) + ":" +
		strconv.Itoa(maxNodes) + ":" + strconv.Itoa(maxLinks)

	// Check cache first
	if cachedData, found := h.cache.Get(key); found {
		metrics.APICacheHits.WithLabelValues("graph_region").Inc()
		span.SetAttributes(attribute.Bool("cache_hit", true))
		w.Header().Set("Content-Type", "application/json")
		w.Write(cachedData)
		return
	}
	metrics.APICacheMisses.WithLabelValues("graph_region").Inc()
	span.SetAttributes(attribute.Bool("cache_hit", false))

	// Fetch nodes in bounding box
	nodesRows, err := h.queries.GetNodesInBoundingBox(ctx, db.GetNodesInBoundingBoxParams{
		PosX:   sql.NullFloat64{Float64: xMin, Valid: true},
		PosX_2: sql.NullFloat64{Float64: xMax, Valid: true},
		PosY:   sql.NullFloat64{Float64: yMin, Valid: true},
		PosY_2: sql.NullFloat64{Float64: yMax, Valid: true},
		PosZ:   sql.NullFloat64{Float64: zMin, Valid: true},
		PosZ_2: sql.NullFloat64{Float64: zMax, Valid: true},
		Limit:  int32(maxNodes),
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			logger.WarnContext(ctx, "region nodes query timed out", "timeout", timeout)
			span.RecordError(err)
			span.SetStatus(codes.Error, "query timeout")
			apierr.WriteErrorWithContext(w, r, apierr.GraphTimeout("Region query timeout"))
			return
		}
		logger.ErrorContext(ctx, "failed to fetch nodes in bounding box", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "query failed")
		apierr.WriteErrorWithContext(w, r, apierr.GraphQueryFailed("Failed to fetch region nodes"))
		return
	}

	// Fetch links for nodes in bounding box
	linksRows, err := h.queries.GetLinksForNodesInBoundingBox(ctx, db.GetLinksForNodesInBoundingBoxParams{
		PosX:   sql.NullFloat64{Float64: xMin, Valid: true},
		PosX_2: sql.NullFloat64{Float64: xMax, Valid: true},
		PosY:   sql.NullFloat64{Float64: yMin, Valid: true},
		PosY_2: sql.NullFloat64{Float64: yMax, Valid: true},
		PosZ:   sql.NullFloat64{Float64: zMin, Valid: true},
		PosZ_2: sql.NullFloat64{Float64: zMax, Valid: true},
		Limit:  int32(maxLinks),
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			logger.WarnContext(ctx, "region links query timed out", "timeout", timeout)
			span.RecordError(err)
			span.SetStatus(codes.Error, "query timeout")
			apierr.WriteErrorWithContext(w, r, apierr.GraphTimeout("Region query timeout"))
			return
		}
		logger.ErrorContext(ctx, "failed to fetch links in bounding box", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "query failed")
		apierr.WriteErrorWithContext(w, r, apierr.GraphQueryFailed("Failed to fetch region links"))
		return
	}

	// Build response
	nodes := make([]GraphNode, 0, len(nodesRows))
	for _, row := range nodesRows {
		valStr := ""
		if row.Val.Valid {
			valStr = row.Val.String
		}
		v := atoiSafe(valStr)
		t := ""
		if row.Type.Valid {
			t = strings.ToLower(row.Type.String)
		}
		gn := GraphNode{
			ID:   row.ID,
			Name: row.Name,
			Val:  v,
			Type: t,
		}
		// Spatial queries always include positions since they're filtered by position
		if row.PosX.Valid && row.PosY.Valid && row.PosZ.Valid {
			x := row.PosX.Float64
			y := row.PosY.Float64
			z := row.PosZ.Float64
			gn.X = &x
			gn.Y = &y
			gn.Z = &z
		}
		nodes = append(nodes, gn)
	}

	links := make([]GraphLink, 0, len(linksRows))
	for _, row := range linksRows {
		src := toString(row.Source)
		tgt := toString(row.Target)
		if src != "" && tgt != "" {
			links = append(links, GraphLink{Source: src, Target: tgt})
		}
	}

	resp := GraphResponse{Nodes: nodes, Links: links}
	b, err := json.Marshal(resp)
	if err != nil {
		logger.ErrorContext(ctx, "failed to marshal region response", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "marshal failed")
		apierr.WriteErrorWithContext(w, r, apierr.SystemInternal("Failed to marshal response"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)

	// Store in cache
	h.cache.Set(key, b, 0)

	span.SetAttributes(
		attribute.Int("nodes_count", len(nodes)),
		attribute.Int("links_count", len(links)),
	)
	span.SetStatus(codes.Ok, "region fetched successfully")
}

// getGraphDataPaginated handles paginated graph data requests
func (h *Handler) getGraphDataPaginated(w http.ResponseWriter, r *http.Request, ctx context.Context, cursorParam, pageSizeParam string, withPos, allowAll bool, allowedTypes map[string]struct{}) {
	ctx, span := tracing.StartSpan(ctx, "handlers.getGraphDataPaginated")
	defer span.End()
	
	// Parse page size (default 5000)
	pageSize := parseIntDefault(pageSizeParam, 5000)
	if pageSize <= 0 {
		pageSize = 5000
	}
	// Cap at reasonable maximum to prevent abuse
	if pageSize > 50000 {
		pageSize = 50000
	}
	
	// Decode cursor
	cursor, err := decodeCursor(cursorParam)
	if err != nil {
		logger.WarnContext(ctx, "Invalid cursor", "cursor", cursorParam, "error", err)
		apierr.WriteErrorWithContext(w, r, apierr.GraphInvalidParams("Invalid cursor format"))
		return
	}
	
	span.SetAttributes(
		attribute.Int("page_size", pageSize),
		attribute.Bool("has_cursor", cursorParam != ""),
		attribute.Bool("with_positions", withPos),
	)
	
	// Fetch page_size + 1 nodes to check if there are more
	var cursorWeight sql.NullInt64
	var cursorID sql.NullString
	
	if cursorParam != "" {
		cursorWeight = sql.NullInt64{Int64: cursor.Weight, Valid: true}
		cursorID = sql.NullString{String: cursor.ID, Valid: true}
	}
	
	rows, err := h.queries.GetPaginatedGraphNodes(ctx, db.GetPaginatedGraphNodesParams{
		Column1: cursorWeight.Int64,
		Column2: cursorID.String,
		Limit:   int32(pageSize + 1),
	})
	
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			logger.WarnContext(ctx, "Paginated query timed out")
			span.RecordError(err)
			span.SetStatus(codes.Error, "query timeout")
			apierr.WriteErrorWithContext(w, r, apierr.GraphTimeout("Query timeout"))
			return
		}
		logger.ErrorContext(ctx, "Failed to fetch paginated nodes", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "query failed")
		apierr.WriteErrorWithContext(w, r, apierr.GraphQueryFailed("Failed to fetch paginated graph data"))
		return
	}
	
	// Check if there are more results
	hasMore := len(rows) > pageSize
	if hasMore {
		rows = rows[:pageSize]
	}
	
	// Build nodes response
	nodes := make([]GraphNode, 0, len(rows))
	nodeIDs := make([]string, 0, len(rows))
	
	for _, row := range rows {
		v := 0
		if row.Val.Valid {
			v = atoiSafe(row.Val.String)
		}
		
		t := ""
		if row.Type.Valid {
			t = strings.ToLower(row.Type.String)
		}
		
		// Apply type filter if needed
		if !allowAll {
			if len(allowedTypes) == 0 {
				continue
			}
			if t == "" {
				continue
			}
			if _, ok := allowedTypes[t]; !ok {
				continue
			}
		}
		
		gn := GraphNode{
			ID:   row.ID,
			Name: row.Name,
			Val:  v,
			Type: t,
		}
		
		if withPos {
			if row.PosX.Valid {
				x := row.PosX.Float64
				gn.X = &x
			}
			if row.PosY.Valid {
				y := row.PosY.Float64
				gn.Y = &y
			}
			if row.PosZ.Valid {
				z := row.PosZ.Float64
				gn.Z = &z
			}
		}
		
		nodes = append(nodes, gn)
		nodeIDs = append(nodeIDs, row.ID)
	}
	
	// Fetch links for these nodes
	// Note: For pagination, we only include links where both endpoints are in the current page
	// This is a simplified approach; a more complete implementation would track seen nodes across pages
	links := make([]GraphLink, 0)
	if len(nodeIDs) > 0 {
		// Default max links for a page
		maxLinksForPage := pageSize * 5
		
		linkRows, err := h.queries.GetLinksForPaginatedNodes(ctx, db.GetLinksForPaginatedNodesParams{
			Column1: nodeIDs,
			Limit:   int32(maxLinksForPage),
		})
		
		if err != nil {
			logger.WarnContext(ctx, "Failed to fetch links for paginated nodes", "error", err)
			// Continue without links rather than failing the whole request
		} else {
			for _, lr := range linkRows {
				src := toString(lr.Source)
				tgt := toString(lr.Target)
				if src != "" && tgt != "" {
					links = append(links, GraphLink{Source: src, Target: tgt})
				}
			}
		}
	}
	
	// Generate next cursor if there are more results
	var nextCursor string
	if hasMore && len(nodes) > 0 {
		lastNode := nodes[len(nodes)-1]
		// Get the weight for the last node
		lastWeight := int64(lastNode.Val)
		nextCursor = encodeCursor(lastWeight, lastNode.ID)
	}
	
	// Build paginated response
	resp := PaginatedGraphResponse{
		Nodes: nodes,
		Links: links,
		Pagination: &PaginationInfo{
			NextCursor: nextCursor,
			HasMore:    hasMore,
			PageSize:   pageSize,
		},
	}
	
	b, err := json.Marshal(resp)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to marshal paginated response", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "marshal failed")
		apierr.WriteErrorWithContext(w, r, apierr.SystemInternal("Failed to marshal response"))
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
	
	// Note: Pagination responses are typically not cached as they are cursor-specific
	// and would require complex cache invalidation logic
	
	span.SetAttributes(
		attribute.Int("nodes_count", len(nodes)),
		attribute.Int("links_count", len(links)),
		attribute.Bool("has_more", hasMore),
	)
	span.SetStatus(codes.Ok, "paginated data fetched successfully")
}
