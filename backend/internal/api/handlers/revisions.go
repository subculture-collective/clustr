package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

type RevisionHandler struct{ db revisionDB }
type revisionDB interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

type revisionNode struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Val                int64           `json:"val"`
	Type               string          `json:"type"`
	X                  float64         `json:"x"`
	Y                  float64         `json:"y"`
	Z                  float64         `json:"z"`
	ParentID           string          `json:"parent_id,omitempty"`
	AnchorID           string          `json:"anchor_id,omitempty"`
	AuthorID           string          `json:"author_id,omitempty"`
	CommunityID        string          `json:"community_id,omitempty"`
	PositionProvenance string          `json:"position_provenance,omitempty"`
	SourceUpdatedAt    string          `json:"source_updated_at,omitempty"`
	Metrics            json.RawMessage `json:"metrics,omitempty"`
}
type revisionLink struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Type     string `json:"type"`
	Directed bool   `json:"directed"`
	Weight   int64  `json:"weight"`
}
type scenePayload struct {
	RevisionID     int64          `json:"revision_id"`
	SpatialCatalog int64          `json:"spatial_catalog_id,omitempty"`
	Scope          string         `json:"scope,omitempty"`
	RootID         string         `json:"root_id,omitempty"`
	Truncated      bool           `json:"truncated,omitempty"`
	Nodes          []revisionNode `json:"nodes"`
	Links          []revisionLink `json:"links"`
	NextCursor     string         `json:"next_cursor,omitempty"`
}

type rowScanner interface {
	Scan(...any) error
}

func scanSpatialNode(scanner rowScanner) (revisionNode, error) {
	var node revisionNode
	var metrics []byte
	err := scanner.Scan(&node.ID, &node.Name, &node.Val, &node.Type, &node.X, &node.Y, &node.Z,
		&node.ParentID, &node.AnchorID, &node.AuthorID, &node.CommunityID,
		&node.PositionProvenance, &node.SourceUpdatedAt, &metrics)
	if len(metrics) > 0 {
		node.Metrics = append(json.RawMessage(nil), metrics...)
	}
	return node, err
}

type entityExpansion struct {
	Name     string
	PageSize int
	MaxTotal int
	Depth    int
}

func parseEntityExpansion(r *http.Request) (entityExpansion, error) {
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("expand")))
	switch name {
	case "":
		return entityExpansion{Name: "neighborhood", PageSize: queryLimit(r, 5000, 25000), MaxTotal: 25000}, nil
	case "system":
		return entityExpansion{Name: name, PageSize: queryLimit(r, 512, 512), MaxTotal: 512}, nil
	case "discussion":
		return entityExpansion{Name: name, PageSize: queryLimit(r, 512, 512), MaxTotal: 2048}, nil
	case "thread":
		depth := 3
		if raw := r.URL.Query().Get("depth"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 3 {
				return entityExpansion{}, errors.New("thread depth must be between 1 and 3")
			}
			depth = value
		}
		return entityExpansion{Name: name, PageSize: queryLimit(r, 250, 250), MaxTotal: 250, Depth: depth}, nil
	default:
		return entityExpansion{}, errors.New("expand must be system, discussion, or thread")
	}
}

func NewRevisionHandler(d revisionDB) *RevisionHandler { return &RevisionHandler{db: d} }

func (h *RevisionHandler) catalogID(ctx context.Context, revisionID int64) (sql.NullInt64, error) {
	var catalog sql.NullInt64
	err := h.db.QueryRowContext(ctx, `SELECT spatial_catalog_id FROM graph_revisions WHERE id=$1 AND status='published'`, revisionID).Scan(&catalog)
	return catalog, err
}

func (h *RevisionHandler) revision(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("revision")
	if raw == "" {
		raw = r.URL.Query().Get("revision_id")
	}
	if raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			return 0, errors.New("invalid revision")
		}
		var published bool
		if err := h.db.QueryRowContext(r.Context(), `SELECT status='published' FROM graph_revisions WHERE id=$1`, id).Scan(&published); err != nil || !published {
			return 0, errors.New("revision unavailable")
		}
		return id, nil
	}
	var id int64
	err := h.db.QueryRowContext(r.Context(), `SELECT revision_id FROM graph_revision_current WHERE singleton`).Scan(&id)
	return id, err
}

func writeRevisionError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

type manifestPayload struct {
	RevisionID             int64              `json:"revision_id"`
	Watermark              string             `json:"source_watermark"`
	Published              string             `json:"published_at"`
	Nodes                  int64              `json:"node_count"`
	ResidentNodes          int64              `json:"resident_node_count"`
	Links                  int64              `json:"link_count"`
	ResidentLinks          int64              `json:"resident_link_count"`
	Communities            int64              `json:"community_count"`
	CatalogID              int64              `json:"spatial_catalog_id,omitempty"`
	Layout                 string             `json:"layout_algorithm"`
	Dimensions             int                `json:"dimensions"`
	Bounds                 map[string]float64 `json:"bounds"`
	Levels                 []int              `json:"hierarchy_levels"`
	Formats                []string           `json:"supported_formats"`
	SceneContract          string             `json:"scene_contract"`
	BotPolicyVersion       string             `json:"bot_policy_version,omitempty"`
	MetricPolicyVersion    string             `json:"metric_policy_version,omitempty"`
	CatalogChecksum        string             `json:"catalog_checksum,omitempty"`
	ExcludedUserCount      int64              `json:"excluded_user_count,omitempty"`
	ExcludedPostCount      int64              `json:"excluded_post_count,omitempty"`
	ExcludedCommentCount   int64              `json:"excluded_comment_count,omitempty"`
	NormalizationQuantiles json.RawMessage    `json:"normalization_quantiles,omitempty"`
}

func (h *RevisionHandler) Manifest(w http.ResponseWriter, r *http.Request) {
	id, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	var out manifestPayload
	var quantiles []byte
	var minX, maxX, minY, maxY, minZ, maxZ float64
	err = h.db.QueryRowContext(r.Context(), `SELECT r.id,r.source_watermark::text,r.published_at::text,
COALESCE(c.entity_count,r.node_count),r.node_count,COALESCE(c.link_count,r.link_count),r.link_count,r.community_count,
r.layout_algorithm,r.layout_dimensions,COALESCE(c.min_x,r.min_x),COALESCE(c.max_x,r.max_x),COALESCE(c.min_y,r.min_y),
COALESCE(c.max_y,r.max_y),COALESCE(c.min_z,r.min_z),COALESCE(c.max_z,r.max_z),COALESCE(c.id,0),
COALESCE(c.bot_policy_version,''),COALESCE(c.metric_policy_version,''),COALESCE(c.catalog_checksum,''),
COALESCE(c.excluded_user_count,0),COALESCE(c.excluded_post_count,0),COALESCE(c.excluded_comment_count,0),
COALESCE(c.config->'normalization_quantiles','{}'::jsonb)
FROM graph_revisions r LEFT JOIN spatial_catalogs c ON c.id=r.spatial_catalog_id AND c.status='published'
WHERE r.id=$1 AND r.status='published'`, id).Scan(
		&out.RevisionID, &out.Watermark, &out.Published, &out.Nodes, &out.ResidentNodes, &out.Links, &out.ResidentLinks,
		&out.Communities, &out.Layout, &out.Dimensions, &minX, &maxX, &minY, &maxY, &minZ, &maxZ, &out.CatalogID,
		&out.BotPolicyVersion, &out.MetricPolicyVersion, &out.CatalogChecksum, &out.ExcludedUserCount, &out.ExcludedPostCount,
		&out.ExcludedCommentCount, &quantiles)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	out.Bounds = map[string]float64{"min_x": minX, "max_x": maxX, "min_y": minY, "max_y": maxY, "min_z": minZ, "max_z": maxZ}
	levelRows, err := h.db.QueryContext(r.Context(), `SELECT DISTINCT level FROM graph_revision_communities WHERE revision_id=$1 ORDER BY level`, id)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer levelRows.Close()
	out.Levels = []int{}
	for levelRows.Next() {
		var level int
		if err := levelRows.Scan(&level); err != nil {
			writeRevisionError(w, err)
			return
		}
		out.Levels = append(out.Levels, level)
	}
	if err := levelRows.Err(); err != nil {
		writeRevisionError(w, err)
		return
	}
	out.Formats = []string{"json"}
	out.SceneContract = "spatial-scene-v1"
	out.NormalizationQuantiles = append(json.RawMessage(nil), quantiles...)
	writeJSON(w, out)
}

func queryLimit(r *http.Request, fallback, ceiling int) int {
	return namedQueryLimit(r, "max_nodes", fallback, ceiling)
}

func namedQueryLimit(r *http.Request, name string, fallback, ceiling int) int {
	value, _ := strconv.Atoi(r.URL.Query().Get(name))
	if value <= 0 {
		value = fallback
	}
	if value > ceiling {
		value = ceiling
	}
	return value
}

func (h *RevisionHandler) Overview(w http.ResponseWriter, r *http.Request) {
	id, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	catalog, err := h.catalogID(r.Context(), id)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	level, _ := strconv.Atoi(r.URL.Query().Get("level"))
	limit := queryLimit(r, 300, 2000)
	linkLimit := namedQueryLimit(r, "max_links", 180, 2000)
	rows, err := h.db.QueryContext(r.Context(), `SELECT community_id,label,size,x,y,z FROM graph_revision_communities WHERE revision_id=$1 AND level=$2 ORDER BY size DESC,community_id LIMIT $3`, id, level, limit)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	payload := scenePayload{RevisionID: id, Nodes: []revisionNode{}, Links: []revisionLink{}}
	if catalog.Valid {
		payload.SpatialCatalog = catalog.Int64
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var n revisionNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Val, &n.X, &n.Y, &n.Z); err != nil {
			writeRevisionError(w, err)
			return
		}
		n.Type = "community"
		payload.Nodes = append(payload.Nodes, n)
		ids = append(ids, n.ID)
	}
	if err := rows.Err(); err != nil {
		writeRevisionError(w, err)
		return
	}
	if len(ids) > 0 {
		linkRows, err := h.db.QueryContext(r.Context(), `SELECT source_community_id,target_community_id,'community_route',false,weight FROM graph_revision_community_links WHERE revision_id=$1 AND source_community_id=ANY($2) AND target_community_id=ANY($2) ORDER BY weight DESC,source_community_id,target_community_id LIMIT $3`, id, stringArray(ids), linkLimit)
		if err != nil {
			writeRevisionError(w, err)
			return
		}
		defer linkRows.Close()
		for linkRows.Next() {
			var link revisionLink
			if err := linkRows.Scan(&link.Source, &link.Target, &link.Type, &link.Directed, &link.Weight); err != nil {
				writeRevisionError(w, err)
				return
			}
			payload.Links = append(payload.Links, link)
		}
	}
	writeJSON(w, payload)
}

// stringArray implements driver.Valuer without importing a database-specific
// handler contract. lib/pq accepts the familiar PostgreSQL array literal.
type stringArray []string

func (a stringArray) Value() (driver.Value, error) {
	parts := make([]string, len(a))
	for i, value := range a {
		parts[i] = `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

type regionCursor struct {
	Revision int64 `json:"revision"`
	Catalog  int64 `json:"catalog,omitempty"`
	Offset   int   `json:"offset"`
}

type entityCursor struct {
	Revision int64  `json:"revision"`
	Catalog  int64  `json:"catalog"`
	RootID   string `json:"root_id"`
	Expand   string `json:"expand"`
	Depth    int    `json:"depth"`
	Offset   int    `json:"offset"`
}

func decodeRegionCursor(raw string) (regionCursor, error) {
	if raw == "" {
		return regionCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return regionCursor{}, errors.New("invalid continuation token")
	}
	var token regionCursor
	if json.Unmarshal(body, &token) != nil || token.Offset < 0 {
		return regionCursor{}, errors.New("invalid continuation token")
	}
	return token, nil
}
func encodeRegionCursor(token regionCursor) string {
	body, _ := json.Marshal(token)
	return base64.RawURLEncoding.EncodeToString(body)
}

func decodeEntityCursor(raw string) (entityCursor, error) {
	if raw == "" {
		return entityCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return entityCursor{}, errors.New("invalid continuation token")
	}
	var token entityCursor
	if json.Unmarshal(body, &token) != nil || token.Offset < 0 || token.Depth < 0 {
		return entityCursor{}, errors.New("invalid continuation token")
	}
	return token, nil
}

func encodeEntityCursor(token entityCursor) string {
	body, _ := json.Marshal(token)
	return base64.RawURLEncoding.EncodeToString(body)
}

func regionBounds(r *http.Request) ([6]float64, error) {
	var bounds [6]float64
	if raw := r.URL.Query().Get("bbox"); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) != 6 {
			return bounds, errors.New("bbox requires min_x,min_y,min_z,max_x,max_y,max_z")
		}
		for i := range parts {
			value, err := strconv.ParseFloat(parts[i], 64)
			if err != nil {
				return bounds, errors.New("bbox must be numeric")
			}
			bounds[i] = value
		}
		return bounds, nil
	}
	aliases := [][2]string{{"min_x", "x_min"}, {"min_y", "y_min"}, {"min_z", "z_min"}, {"max_x", "x_max"}, {"max_y", "y_max"}, {"max_z", "z_max"}}
	for i, names := range aliases {
		raw := r.URL.Query().Get(names[0])
		if raw == "" {
			raw = r.URL.Query().Get(names[1])
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return bounds, errors.New("six numeric region bounds required")
		}
		bounds[i] = value
	}
	return bounds, nil
}

func (h *RevisionHandler) Region(w http.ResponseWriter, r *http.Request) {
	id, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	token, err := decodeRegionCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if token.Revision != 0 && token.Revision != id {
		http.Error(w, "continuation belongs to a different revision", http.StatusConflict)
		return
	}
	catalog, err := h.catalogID(r.Context(), id)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if token.Catalog != 0 && (!catalog.Valid || token.Catalog != catalog.Int64) {
		http.Error(w, "continuation belongs to a different spatial catalog", http.StatusConflict)
		return
	}
	bounds, err := regionBounds(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if bounds[0] > bounds[3] || bounds[1] > bounds[4] || bounds[2] > bounds[5] {
		writeRevisionError(w, errors.New("region minimum exceeds maximum"))
		return
	}
	limit := queryLimit(r, 10000, 25000)
	linkLimit := namedQueryLimit(r, "max_links", 20000, 50000)
	lod := strings.ToLower(r.URL.Query().Get("lod"))
	if lod == "" {
		lod = "near"
	}
	typeFilter := "true"
	switch lod {
	case "medium":
		typeFilter = `type IN ('subreddit','user')`
	case "near":
		typeFilter = `type IN ('subreddit','user')`
	case "inspect":
		// Comments are revealed through the selected post/entity neighborhood.
		// Keeping them out of free-camera region scans avoids a multi-million-row
		// 3D index whose write amplification dominates catalog publication.
		typeFilter = `type IN ('subreddit','user','post')`
	default:
		writeRevisionError(w, errors.New("lod must be medium, near, or inspect"))
		return
	}
	table := "graph_revision_nodes"
	key := "revision_id"
	if catalog.Valid {
		table = "spatial_catalog_entities"
		key = "catalog_id"
	}
	columns := "id,name,value,type,x,y,z"
	if catalog.Valid {
		columns = "id,label,value,type,x,y,z,COALESCE(parent_id,''),COALESCE(anchor_id,''),COALESCE(author_id,''),COALESCE(community_id,''),position_provenance,source_updated_at::text,metrics"
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s=$1 AND x BETWEEN $2 AND $5 AND y BETWEEN $3 AND $6 AND z BETWEEN $4 AND $7 AND %s ORDER BY value DESC,id OFFSET $8 LIMIT $9`, columns, table, key, typeFilter)
	artifactID := id
	if catalog.Valid {
		artifactID = catalog.Int64
	}
	rows, err := h.db.QueryContext(r.Context(), query, artifactID, bounds[0], bounds[1], bounds[2], bounds[3], bounds[4], bounds[5], token.Offset, limit)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	payload := scenePayload{RevisionID: id, Nodes: []revisionNode{}, Links: []revisionLink{}}
	if catalog.Valid {
		payload.SpatialCatalog = catalog.Int64
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var node revisionNode
		if catalog.Valid {
			node, err = scanSpatialNode(rows)
		} else {
			err = rows.Scan(&node.ID, &node.Name, &node.Val, &node.Type, &node.X, &node.Y, &node.Z)
		}
		if err != nil {
			writeRevisionError(w, err)
			return
		}
		payload.Nodes = append(payload.Nodes, node)
		ids = append(ids, node.ID)
	}
	if len(payload.Nodes) == limit {
		payload.NextCursor = encodeRegionCursor(regionCursor{Revision: id, Catalog: payload.SpatialCatalog, Offset: token.Offset + limit})
	}
	if err := h.appendLinks(r.Context(), &payload, ids, linkLimit); err != nil {
		writeRevisionError(w, err)
		return
	}
	writeJSON(w, payload)
}

func (h *RevisionHandler) appendLinks(ctx context.Context, payload *scenePayload, ids []string, limit int) error {
	if len(ids) == 0 {
		return nil
	}
	table := "graph_revision_links"
	key := "revision_id"
	artifactID := payload.RevisionID
	if payload.SpatialCatalog != 0 {
		rows, err := h.db.QueryContext(ctx, `WITH visible AS MATERIALIZED (
 SELECT unnest($2::text[]) id
), materialized AS (
 SELECT link.source,link.target,link.relation,link.directed,link.weight
 FROM visible source
 JOIN spatial_catalog_links link ON link.catalog_id=$1 AND link.source=source.id
 JOIN visible target ON target.id=link.target
), structural AS (
 SELECT author_id source,id target,'authorship' relation,true directed,1::bigint weight
 FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=ANY($2) AND author_id=ANY($2)
 UNION ALL
 SELECT id,parent_id,'published_in',true,1::bigint
 FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=ANY($2) AND type='post' AND parent_id=ANY($2)
 UNION ALL
 SELECT id,anchor_id,'comment_on',true,1::bigint
 FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=ANY($2) AND type='comment' AND anchor_id=ANY($2)
 UNION ALL
 SELECT parent_id,id,'reply',true,1::bigint
 FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=ANY($2) AND type='comment' AND parent_id LIKE 'comment_%' AND parent_id=ANY($2)
)
SELECT source,target,relation,directed,weight FROM (
 SELECT * FROM materialized UNION ALL SELECT * FROM structural
) links ORDER BY weight DESC,source,target,relation LIMIT $3`, payload.SpatialCatalog, stringArray(ids), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var link revisionLink
			if err := rows.Scan(&link.Source, &link.Target, &link.Type, &link.Directed, &link.Weight); err != nil {
				return err
			}
			payload.Links = append(payload.Links, link)
		}
		return rows.Err()
	}
	query := fmt.Sprintf(`SELECT source,target,relation,directed,weight FROM %s WHERE %s=$1 AND source=ANY($2) AND target=ANY($2) ORDER BY weight DESC,source,target,relation LIMIT $3`, table, key)
	rows, err := h.db.QueryContext(ctx, query, artifactID, stringArray(ids), limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var link revisionLink
		if err := rows.Scan(&link.Source, &link.Target, &link.Type, &link.Directed, &link.Weight); err != nil {
			return err
		}
		payload.Links = append(payload.Links, link)
	}
	return rows.Err()
}

func (h *RevisionHandler) Community(w http.ResponseWriter, r *http.Request) {
	id, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	cid := mux.Vars(r)["stable_id"]
	if cid == "" {
		cid = r.URL.Query().Get("community_id")
	}
	limit := queryLimit(r, 5000, 25000)
	linkLimit := namedQueryLimit(r, "max_links", 20000, 50000)
	lod := strings.ToLower(r.URL.Query().Get("lod"))
	if lod == "" {
		lod = "near"
	}
	typeFilter := `type IN ('subreddit','user')`
	if lod == "inspect" {
		typeFilter = `type IN ('subreddit','user','post')`
	} else if lod != "medium" && lod != "near" {
		writeRevisionError(w, errors.New("lod must be medium, near, or inspect"))
		return
	}
	catalog, err := h.catalogID(r.Context(), id)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	var rows *sql.Rows
	if catalog.Valid {
		rows, err = h.db.QueryContext(r.Context(), fmt.Sprintf(`SELECT id,label,value,type,x,y,z,COALESCE(parent_id,''),COALESCE(anchor_id,''),COALESCE(author_id,''),COALESCE(community_id,''),position_provenance,source_updated_at::text,metrics FROM spatial_catalog_entities
WHERE catalog_id=$1 AND community_id=$2 AND %s ORDER BY value DESC,id LIMIT $3`, typeFilter), catalog.Int64, cid, limit)
	} else {
		rows, err = h.db.QueryContext(r.Context(), `SELECT n.id,n.name,n.value,n.type,n.x,n.y,n.z FROM graph_revision_community_members m JOIN graph_revision_nodes n ON n.revision_id=m.revision_id AND n.id=m.node_id WHERE m.revision_id=$1 AND m.community_id=$2 ORDER BY n.value DESC,n.id LIMIT $3`, id, cid, limit)
	}
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	payload := scenePayload{RevisionID: id, Nodes: []revisionNode{}, Links: []revisionLink{}}
	if catalog.Valid {
		payload.SpatialCatalog = catalog.Int64
	}
	ids := []string{}
	for rows.Next() {
		var node revisionNode
		if catalog.Valid {
			node, err = scanSpatialNode(rows)
		} else {
			err = rows.Scan(&node.ID, &node.Name, &node.Val, &node.Type, &node.X, &node.Y, &node.Z)
		}
		if err != nil {
			writeRevisionError(w, err)
			return
		}
		payload.Nodes = append(payload.Nodes, node)
		ids = append(ids, node.ID)
	}
	if err := h.appendLinks(r.Context(), &payload, ids, linkLimit); err != nil {
		writeRevisionError(w, err)
		return
	}
	writeJSON(w, payload)
}

// Entity returns a complete, bounded selectable neighborhood centered on one
// spatial entity. Search-to-flight uses this when the entity is not resident in
// the current camera region.
func (h *RevisionHandler) Entity(w http.ResponseWriter, r *http.Request) {
	id, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	catalog, err := h.catalogID(r.Context(), id)
	if err != nil || !catalog.Valid {
		writeRevisionError(w, errors.New("full spatial catalog is unavailable for this revision"))
		return
	}
	entityID := mux.Vars(r)["id"]
	if entityID == "" {
		writeRevisionError(w, errors.New("entity id is required"))
		return
	}
	expansion, err := parseEntityExpansion(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if requested := r.URL.Query().Get("spatial_catalog_id"); requested != "" {
		requestedID, parseErr := strconv.ParseInt(requested, 10, 64)
		if parseErr != nil || requestedID != catalog.Int64 {
			http.Error(w, "request belongs to a different spatial catalog", http.StatusConflict)
			return
		}
	}
	token, err := decodeEntityCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if token.Revision != 0 && (token.Revision != id || token.Catalog != catalog.Int64 || token.RootID != entityID || token.Expand != expansion.Name || token.Depth != expansion.Depth) {
		http.Error(w, "continuation belongs to a different entity request", http.StatusConflict)
		return
	}
	if token.Offset >= expansion.MaxTotal {
		writeRevisionError(w, errors.New("continuation exceeds the bounded entity scope"))
		return
	}
	limit := expansion.PageSize
	if remaining := expansion.MaxTotal - token.Offset; limit > remaining {
		limit = remaining
	}
	linkLimit := namedQueryLimit(r, "max_links", 20000, 50000)
	columns := `n.id,n.label,n.value,n.type,n.x,n.y,n.z,COALESCE(n.parent_id,''),COALESCE(n.anchor_id,''),COALESCE(n.author_id,''),COALESCE(n.community_id,''),n.position_provenance,n.source_updated_at::text,n.metrics`
	var query string
	var args []any
	switch expansion.Name {
	case "system":
		query = fmt.Sprintf(`WITH page AS (
 SELECT id FROM spatial_catalog_entities WHERE catalog_id=$1 AND type='post' AND parent_id=$2
 ORDER BY value DESC,id OFFSET $3 LIMIT $4
), selected AS (
 SELECT $2::text id UNION SELECT id FROM page UNION
 SELECT DISTINCT author_id FROM spatial_catalog_entities WHERE catalog_id=$1 AND id IN (SELECT id FROM page) AND author_id IS NOT NULL
)
SELECT %s FROM selected JOIN spatial_catalog_entities n ON n.catalog_id=$1 AND n.id=selected.id
ORDER BY (n.id=$2) DESC,n.type,n.value DESC,n.id`, columns)
		args = []any{catalog.Int64, entityID, token.Offset, limit}
	case "discussion":
		query = fmt.Sprintf(`WITH page AS (
 SELECT id FROM spatial_catalog_entities WHERE catalog_id=$1 AND type='comment' AND parent_id=$2
 ORDER BY value DESC,id OFFSET $3 LIMIT $4
), selected AS (SELECT $2::text id UNION SELECT id FROM page)
SELECT %s FROM selected JOIN spatial_catalog_entities n ON n.catalog_id=$1 AND n.id=selected.id
ORDER BY (n.id=$2) DESC,n.value DESC,n.id`, columns)
		args = []any{catalog.Int64, entityID, token.Offset, limit}
	case "thread":
		query = fmt.Sprintf(`WITH RECURSIVE descendants AS (
 SELECT id,0 depth FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2
 UNION ALL
 SELECT child.id,parent.depth+1 FROM descendants parent
 JOIN spatial_catalog_entities child ON child.catalog_id=$1 AND child.parent_id=parent.id
 WHERE parent.depth<$3
), page AS (
 SELECT d.id FROM descendants d JOIN spatial_catalog_entities e ON e.catalog_id=$1 AND e.id=d.id
 WHERE d.depth>0 ORDER BY d.depth,e.value DESC,e.id OFFSET $4 LIMIT $5
), selected AS (SELECT $2::text id UNION SELECT id FROM page)
SELECT %s FROM selected JOIN spatial_catalog_entities n ON n.catalog_id=$1 AND n.id=selected.id
ORDER BY (n.id=$2) DESC,n.value DESC,n.id`, columns)
		args = []any{catalog.Int64, entityID, expansion.Depth, token.Offset, limit}
	default:
		query = fmt.Sprintf(`WITH adjacent AS (
 SELECT target neighbor_id,weight FROM spatial_catalog_links WHERE catalog_id=$1 AND source=$2
 UNION ALL
 SELECT source neighbor_id,weight FROM spatial_catalog_links WHERE catalog_id=$1 AND target=$2
 UNION ALL
 SELECT DISTINCT ref,1::bigint FROM spatial_catalog_entities e
 CROSS JOIN LATERAL unnest(ARRAY[e.parent_id,e.anchor_id,e.author_id]) ref
 WHERE e.catalog_id=$1 AND e.id=$2 AND ref IS NOT NULL
 UNION ALL
 SELECT id,1::bigint FROM spatial_catalog_entities
 WHERE catalog_id=$1 AND (parent_id=$2 OR anchor_id=$2 OR author_id=$2)
), selected AS (
 SELECT $2::text id,9223372036854775807::bigint priority
 UNION ALL
 SELECT neighbor_id,max(weight) priority FROM adjacent WHERE neighbor_id<>$2 GROUP BY neighbor_id
 ORDER BY priority DESC,id LIMIT $3
)
SELECT %s FROM selected
JOIN spatial_catalog_entities n ON n.catalog_id=$1 AND n.id=selected.id
ORDER BY selected.priority DESC,n.value DESC,n.id`, columns)
		args = []any{catalog.Int64, entityID, limit}
	}
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	payload := scenePayload{RevisionID: id, SpatialCatalog: catalog.Int64, Scope: expansion.Name, RootID: entityID, Nodes: []revisionNode{}, Links: []revisionLink{}}
	ids := make([]string, 0, limit)
	childCount := 0
	for rows.Next() {
		node, scanErr := scanSpatialNode(rows)
		if scanErr != nil {
			writeRevisionError(w, scanErr)
			return
		}
		payload.Nodes = append(payload.Nodes, node)
		ids = append(ids, node.ID)
		if node.ID != entityID && ((expansion.Name == "system" && node.Type == "post") || (expansion.Name == "discussion" && node.Type == "comment") || expansion.Name == "thread") {
			childCount++
		}
	}
	if len(payload.Nodes) == 0 {
		http.Error(w, "entity not found", http.StatusNotFound)
		return
	}
	if expansion.Name != "neighborhood" && childCount == limit && token.Offset+limit < expansion.MaxTotal {
		payload.Truncated = true
		payload.NextCursor = encodeEntityCursor(entityCursor{Revision: id, Catalog: catalog.Int64, RootID: entityID, Expand: expansion.Name, Depth: expansion.Depth, Offset: token.Offset + limit})
	}
	if err := h.appendLinks(r.Context(), &payload, ids, linkLimit); err != nil {
		writeRevisionError(w, err)
		return
	}
	writeJSON(w, payload)
}

func (h *RevisionHandler) Diff(w http.ResponseWriter, r *http.Request) {
	fromRaw := r.URL.Query().Get("from_revision")
	if fromRaw == "" {
		fromRaw = r.URL.Query().Get("from_revision_id")
	}
	from, err := strconv.ParseInt(fromRaw, 10, 64)
	if err != nil {
		writeRevisionError(w, errors.New("from_revision is required"))
		return
	}
	to, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if from == to {
		writeJSON(w, map[string]any{"source_revision_id": from, "destination_revision_id": to, "changes": []any{}})
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `
WITH source AS (SELECT id FROM graph_revision_nodes WHERE revision_id=$1), destination AS (SELECT id FROM graph_revision_nodes WHERE revision_id=$2)
SELECT COALESCE(source.id,destination.id),CASE WHEN source.id IS NULL THEN 'add' WHEN destination.id IS NULL THEN 'remove' END
FROM source FULL JOIN destination USING(id) WHERE source.id IS NULL OR destination.id IS NULL ORDER BY 2,1`, from, to)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	changes := []map[string]string{}
	for rows.Next() {
		var entity, action string
		if err := rows.Scan(&entity, &action); err != nil {
			writeRevisionError(w, err)
			return
		}
		changes = append(changes, map[string]string{"entity": "node", "id": entity, "action": action})
	}
	writeJSON(w, map[string]any{"source_revision_id": from, "destination_revision_id": to, "changes": changes})
}

type spatialSearchResult struct {
	ID   string          `json:"ID"`
	Name string          `json:"Name"`
	Val  string          `json:"Val"`
	Type sql.NullString  `json:"Type"`
	PosX sql.NullFloat64 `json:"PosX"`
	PosY sql.NullFloat64 `json:"PosY"`
	PosZ sql.NullFloat64 `json:"PosZ"`
}

// Search exposes semantic labels from the full Spatial Catalog while keeping
// the established frontend response shape during the compatibility window.
func (h *RevisionHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("node"))
	if query == "" {
		writeRevisionError(w, errors.New("node parameter is required"))
		return
	}
	limit := namedQueryLimit(r, "limit", 50, 500)
	// Treat SQL pattern metacharacters as label text. Apart from producing
	// surprising results, an unescaped "%" would turn this bounded search into
	// a full-catalog scan and sort.
	prefix := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	revisionID, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	catalog, err := h.catalogID(r.Context(), revisionID)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	var rows *sql.Rows
	if catalog.Valid {
		rows, err = h.db.QueryContext(r.Context(), `SELECT id,label,value::text,type,x,y,z FROM (
  SELECT id,label,value,type,x,y,z,true AS exact_match
  FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2
  UNION ALL
  (SELECT id,label,value,type,x,y,z,false AS exact_match
   FROM spatial_catalog_entities WHERE catalog_id=$1 AND type IN ('subreddit','user','post')
     AND lower(left(label,128)) LIKE lower($3) ESCAPE '\' AND id<>$2
   ORDER BY lower(left(label,128)),id LIMIT $4)
) results ORDER BY exact_match DESC,value DESC,id LIMIT $4`, catalog.Int64, query, prefix+"%", limit)
	} else {
		rows, err = h.db.QueryContext(r.Context(), `SELECT id,name,value::text,type,x,y,z FROM (
  SELECT id,name,value,type,x,y,z,true AS exact_match
  FROM graph_revision_nodes WHERE revision_id=$1 AND id=$2
  UNION ALL
  (SELECT id,name,value,type,x,y,z,false AS exact_match
   FROM graph_revision_nodes WHERE revision_id=$1 AND lower(left(name,128)) LIKE lower($3) ESCAPE '\' AND id<>$2
   ORDER BY lower(left(name,128)),id LIMIT $4)
) results ORDER BY exact_match DESC,value DESC,id LIMIT $4`, revisionID, query, prefix+"%", limit)
	}
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	results := make([]spatialSearchResult, 0, limit)
	for rows.Next() {
		var result spatialSearchResult
		var kind string
		var x, y, z float64
		if err := rows.Scan(&result.ID, &result.Name, &result.Val, &kind, &x, &y, &z); err != nil {
			writeRevisionError(w, err)
			return
		}
		result.Type = sql.NullString{String: kind, Valid: true}
		result.PosX = sql.NullFloat64{Float64: x, Valid: true}
		result.PosY = sql.NullFloat64{Float64: y, Valid: true}
		result.PosZ = sql.NullFloat64{Float64: z, Valid: true}
		results = append(results, result)
	}
	writeJSON(w, map[string]any{"query": query, "count": len(results), "revision_id": revisionID, "results": results})
}

// NodeDetails reads one full-corpus entity and a bounded, typed neighborhood.
// It intentionally uses the same NodeDetailResponse as the legacy Adapter.
func (h *RevisionHandler) NodeDetails(w http.ResponseWriter, r *http.Request) {
	nodeID := mux.Vars(r)["id"]
	if nodeID == "" {
		writeRevisionError(w, errors.New("node id is required"))
		return
	}
	revisionID, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	catalog, err := h.catalogID(r.Context(), revisionID)
	if err != nil || !catalog.Valid {
		writeRevisionError(w, errors.New("full spatial catalog is unavailable for this revision"))
		return
	}
	neighborLimit := namedQueryLimit(r, "neighbor_limit", 20, 100)
	var response NodeDetailResponse
	var value int64
	var x, y, z float64
	err = h.db.QueryRowContext(r.Context(), `SELECT id,label,value,type,x,y,z FROM spatial_catalog_entities
WHERE catalog_id=$1 AND id=$2`, catalog.Int64, nodeID).Scan(&response.ID, &response.Name, &value, &response.Type, &x, &y, &z)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	response.Val = strconv.FormatInt(value, 10)
	response.PosX, response.PosY, response.PosZ = &x, &y, &z
	rows, err := h.db.QueryContext(r.Context(), `WITH adjacent AS (
 SELECT target neighbor_id FROM spatial_catalog_links WHERE catalog_id=$1 AND source=$2
 UNION ALL
 SELECT source neighbor_id FROM spatial_catalog_links WHERE catalog_id=$1 AND target=$2
 UNION ALL
 SELECT DISTINCT ref FROM spatial_catalog_entities e
 CROSS JOIN LATERAL unnest(ARRAY[e.parent_id,e.anchor_id,e.author_id]) ref
 WHERE e.catalog_id=$1 AND e.id=$2 AND ref IS NOT NULL
 UNION ALL
 SELECT id FROM spatial_catalog_entities
 WHERE catalog_id=$1 AND (parent_id=$2 OR anchor_id=$2 OR author_id=$2)
), ranked AS (
 SELECT neighbor_id,count(*) degree FROM adjacent GROUP BY neighbor_id ORDER BY degree DESC,neighbor_id LIMIT $3
)
SELECT n.id,n.label,n.value::text,n.type,ranked.degree FROM ranked
JOIN spatial_catalog_entities n ON n.catalog_id=$1 AND n.id=ranked.neighbor_id
ORDER BY ranked.degree DESC,n.value DESC,n.id`, catalog.Int64, nodeID, neighborLimit)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var neighbor NeighborInfo
		if err := rows.Scan(&neighbor.ID, &neighbor.Name, &neighbor.Val, &neighbor.Type, &neighbor.Degree); err != nil {
			writeRevisionError(w, err)
			return
		}
		response.Neighbors = append(response.Neighbors, neighbor)
	}
	response.Degree = len(response.Neighbors)
	writeJSON(w, response)
}
