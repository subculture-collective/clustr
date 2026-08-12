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
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Val  int64   `json:"val"`
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
}
type revisionLink struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Type     string `json:"type"`
	Directed bool   `json:"directed"`
	Weight   int64  `json:"weight"`
}
type scenePayload struct {
	RevisionID int64          `json:"revision_id"`
	Nodes      []revisionNode `json:"nodes"`
	Links      []revisionLink `json:"links"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func NewRevisionHandler(d revisionDB) *RevisionHandler { return &RevisionHandler{db: d} }

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

func (h *RevisionHandler) Manifest(w http.ResponseWriter, r *http.Request) {
	id, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	var out struct {
		RevisionID    int64              `json:"revision_id"`
		Watermark     string             `json:"source_watermark"`
		Published     string             `json:"published_at"`
		Nodes         int64              `json:"node_count"`
		Links         int64              `json:"link_count"`
		Communities   int64              `json:"community_count"`
		Layout        string             `json:"layout_algorithm"`
		Dimensions    int                `json:"dimensions"`
		Bounds        map[string]float64 `json:"bounds"`
		Levels        []int              `json:"hierarchy_levels"`
		Formats       []string           `json:"supported_formats"`
		SceneContract string             `json:"scene_contract"`
	}
	var minX, maxX, minY, maxY, minZ, maxZ float64
	err = h.db.QueryRowContext(r.Context(), `SELECT id,source_watermark::text,published_at::text,node_count,link_count,community_count,layout_algorithm,layout_dimensions,min_x,max_x,min_y,max_y,min_z,max_z FROM graph_revisions WHERE id=$1 AND status='published'`, id).Scan(
		&out.RevisionID, &out.Watermark, &out.Published, &out.Nodes, &out.Links, &out.Communities, &out.Layout, &out.Dimensions, &minX, &maxX, &minY, &maxY, &minZ, &maxZ)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	out.Bounds = map[string]float64{"min_x": minX, "max_x": maxX, "min_y": minY, "max_y": maxY, "min_z": minZ, "max_z": maxZ}
	out.Levels = []int{0, 1, 2, 3}
	out.Formats = []string{"json"}
	out.SceneContract = "spatial-scene-v1"
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
	Offset   int   `json:"offset"`
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
		typeFilter = "true"
	default:
		writeRevisionError(w, errors.New("lod must be medium, near, or inspect"))
		return
	}
	query := fmt.Sprintf(`SELECT id,name,value,type,x,y,z FROM graph_revision_nodes WHERE revision_id=$1 AND x BETWEEN $2 AND $5 AND y BETWEEN $3 AND $6 AND z BETWEEN $4 AND $7 AND %s ORDER BY value DESC,id OFFSET $8 LIMIT $9`, typeFilter)
	rows, err := h.db.QueryContext(r.Context(), query, id, bounds[0], bounds[1], bounds[2], bounds[3], bounds[4], bounds[5], token.Offset, limit)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	payload := scenePayload{RevisionID: id, Nodes: []revisionNode{}, Links: []revisionLink{}}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var node revisionNode
		if err := rows.Scan(&node.ID, &node.Name, &node.Val, &node.Type, &node.X, &node.Y, &node.Z); err != nil {
			writeRevisionError(w, err)
			return
		}
		payload.Nodes = append(payload.Nodes, node)
		ids = append(ids, node.ID)
	}
	if len(payload.Nodes) == limit {
		payload.NextCursor = encodeRegionCursor(regionCursor{Revision: id, Offset: token.Offset + limit})
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
	rows, err := h.db.QueryContext(ctx, `SELECT source,target,relation,directed,weight FROM graph_revision_links WHERE revision_id=$1 AND source=ANY($2) AND target=ANY($2) ORDER BY weight DESC,source,target,relation LIMIT $3`, payload.RevisionID, stringArray(ids), limit)
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
	rows, err := h.db.QueryContext(r.Context(), `SELECT n.id,n.name,n.value,n.type,n.x,n.y,n.z FROM graph_revision_community_members m JOIN graph_revision_nodes n ON n.revision_id=m.revision_id AND n.id=m.node_id WHERE m.revision_id=$1 AND m.community_id=$2 ORDER BY n.value DESC,n.id LIMIT $3`, id, cid, limit)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	payload := scenePayload{RevisionID: id, Nodes: []revisionNode{}, Links: []revisionLink{}}
	ids := []string{}
	for rows.Next() {
		var node revisionNode
		if err := rows.Scan(&node.ID, &node.Name, &node.Val, &node.Type, &node.X, &node.Y, &node.Z); err != nil {
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
