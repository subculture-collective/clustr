package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

type catalogItem struct {
	ID                 string          `json:"id"`
	Type               string          `json:"type"`
	Label              string          `json:"label"`
	Value              int64           `json:"value"`
	ParentID           string          `json:"parent_id,omitempty"`
	AuthorID           string          `json:"author_id,omitempty"`
	CommunityID        string          `json:"community_id,omitempty"`
	MacroGroupID       string          `json:"macro_group_id,omitempty"`
	Title              string          `json:"title,omitempty"`
	Body               string          `json:"body,omitempty"`
	Description        string          `json:"description,omitempty"`
	SourcePermalink    string          `json:"source_permalink,omitempty"`
	Sensitive          bool            `json:"sensitive"`
	SensitivitySources []string        `json:"sensitivity_sources,omitempty"`
	UpdatedAt          string          `json:"updated_at"`
	Metrics            json.RawMessage `json:"metrics"`
	Distinctiveness    float64         `json:"distinctiveness"`
	AffinityConfidence float64         `json:"affinity_confidence"`
	Snippet            string          `json:"snippet,omitempty"`
}

type catalogPayload struct {
	RevisionID     int64          `json:"revision_id"`
	SpatialCatalog int64          `json:"spatial_catalog_id"`
	ReturnedCount  int            `json:"returned_count"`
	TotalCount     int64          `json:"total_count"`
	Items          []catalogItem  `json:"items"`
	NextCursor     string         `json:"next_cursor,omitempty"`
	AppliedFilters map[string]any `json:"applied_filters"`
	Partial        bool           `json:"partial"`
	Stale          bool           `json:"stale"`
}

var catalogTypes = map[string]bool{"subreddit": true, "user": true, "post": true, "comment": true}
var catalogSorts = map[string]string{
	"relevance":       "relevance DESC,e.value DESC,e.id",
	"activity":        "COALESCE((e.metrics->>'activity_count')::bigint,e.value) DESC,e.id",
	"size":            "e.value DESC,e.id",
	"distinctiveness": "distinctiveness DESC,e.value DESC,e.id",
	"newest":          "e.content_updated_at DESC NULLS LAST,e.id",
}

// Catalog returns one revision-pinned, filter-bound page from the complete
// immutable public catalog. Active suppressions are applied at read time.
func (h *RevisionHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	revisionID, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	catalog, err := h.catalogID(r.Context(), revisionID)
	if err != nil || !catalog.Valid {
		writeRevisionError(w, errors.New("published spatial catalog unavailable"))
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	kind := strings.TrimSpace(r.URL.Query().Get("type"))
	if kind != "" && !catalogTypes[kind] {
		writeRevisionError(w, errors.New("type must be subreddit, user, post, or comment"))
		return
	}
	sortName := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortName == "" {
		if query == "" {
			sortName = "activity"
		} else {
			sortName = "relevance"
		}
	}
	orderBy, ok := catalogSorts[sortName]
	if !ok {
		writeRevisionError(w, errors.New("sort must be relevance, activity, size, distinctiveness, or newest"))
		return
	}
	sensitive := strings.TrimSpace(r.URL.Query().Get("sensitive"))
	if sensitive != "" && sensitive != "true" && sensitive != "false" {
		writeRevisionError(w, errors.New("sensitive must be true or false"))
		return
	}
	limit := namedQueryLimit(r, "limit", 50, 200)
	token, err := decodeCatalogCursor(r.URL.Query().Get("cursor"), revisionID, catalog.Int64, r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	communityID := strings.TrimSpace(r.URL.Query().Get("community_id"))
	macroGroupID := strings.TrimSpace(r.URL.Query().Get("macro_group_id"))
	parentID := strings.TrimSpace(r.URL.Query().Get("parent_id"))
	authorID := strings.TrimSpace(r.URL.Query().Get("author_id"))
	relation := strings.TrimSpace(r.URL.Query().Get("relation"))
	entityID := strings.TrimSpace(r.URL.Query().Get("entity_id"))
	sqlQuery := fmt.Sprintf(`WITH matching AS (
 SELECT e.id,e.type,e.label,e.value,COALESCE(e.parent_id,'') parent_id,COALESCE(e.author_id,'') author_id,COALESCE(e.community_id,'') community_id,
  COALESCE(c.macro_group_id,'') macro_group_id,COALESCE(d.title,'') title,COALESCE(d.body,'') body,COALESCE(d.description,'') description,
  COALESCE(d.source_permalink,'') source_permalink,COALESCE(d.source_sensitive,false) source_sensitive,COALESCE(d.administrative_sensitive,false) administrative_sensitive,
  COALESCE(d.content_updated_at,e.source_updated_at)::text content_updated_at,e.metrics,
  COALESCE(c.distinctiveness,0) distinctiveness,COALESCE(c.affinity_confidence,0) affinity_confidence,
  CASE WHEN $3='' THEN 0 ELSE
    ts_rank_cd(COALESCE(d.search_vector,to_tsvector('simple','')),plainto_tsquery('simple',$3)) +
    CASE WHEN lower(e.label)=lower($3) THEN 2 WHEN lower(e.label) LIKE lower($3)||'%%' THEN 1 ELSE 0 END END relevance
 FROM spatial_catalog_entities e
 LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id
 LEFT JOIN graph_revision_communities c ON c.revision_id=$2 AND c.community_id=e.community_id
 WHERE e.catalog_id=$1
  AND NOT EXISTS (SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)
  AND ($3='' OR d.search_vector @@ plainto_tsquery('simple',$3) OR lower(e.label) LIKE '%%'||lower($3)||'%%')
  AND ($4='' OR e.type=$4) AND ($5='' OR e.community_id=$5) AND ($6='' OR c.macro_group_id=$6)
  AND ($7='' OR e.parent_id=$7) AND ($8='' OR e.author_id=$8)
  AND ($9='' OR EXISTS (SELECT 1 FROM spatial_catalog_links l WHERE l.catalog_id=e.catalog_id AND l.relation=$9 AND (l.source=e.id OR l.target=e.id)))
  AND ($10='' OR (d.source_sensitive OR d.administrative_sensitive)=($10='true'))
  AND ($11='' OR e.id=$11)
)
SELECT id,type,label,value,parent_id,author_id,community_id,macro_group_id,title,body,description,source_permalink,
 source_sensitive,administrative_sensitive,content_updated_at,metrics,distinctiveness,affinity_confidence,relevance
FROM matching e
ORDER BY %s OFFSET $12 LIMIT $13`, orderBy)
	var totalCount int64
	err = h.db.QueryRowContext(r.Context(), `SELECT count(*) FROM spatial_catalog_entities e
LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id
LEFT JOIN graph_revision_communities c ON c.revision_id=$2 AND c.community_id=e.community_id
WHERE e.catalog_id=$1 AND NOT EXISTS (SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)
AND ($3='' OR d.search_vector @@ plainto_tsquery('simple',$3) OR lower(e.label) LIKE '%'||lower($3)||'%')
AND ($4='' OR e.type=$4) AND ($5='' OR e.community_id=$5) AND ($6='' OR c.macro_group_id=$6)
AND ($7='' OR e.parent_id=$7) AND ($8='' OR e.author_id=$8)
AND ($9='' OR EXISTS (SELECT 1 FROM spatial_catalog_links l WHERE l.catalog_id=e.catalog_id AND l.relation=$9 AND (l.source=e.id OR l.target=e.id)))
AND ($10='' OR (d.source_sensitive OR d.administrative_sensitive)=($10='true')) AND ($11='' OR e.id=$11)`,
		catalog.Int64, revisionID, query, kind, communityID, macroGroupID, parentID, authorID, relation, sensitive, entityID).Scan(&totalCount)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	rows, err := h.db.QueryContext(r.Context(), sqlQuery, catalog.Int64, revisionID, query, kind, communityID, macroGroupID, parentID, authorID, relation, sensitive, entityID, token.Offset, limit+1)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	out := catalogPayload{RevisionID: revisionID, SpatialCatalog: catalog.Int64, Items: []catalogItem{}, AppliedFilters: map[string]any{
		"q": query, "type": kind, "community_id": communityID, "macro_group_id": macroGroupID,
		"parent_id": parentID, "author_id": authorID, "relation": relation, "sensitive": sensitive, "entity_id": entityID, "sort": sortName,
	}}
	out.TotalCount = totalCount
	for rows.Next() {
		var item catalogItem
		var sourceSensitive, administrativeSensitive bool
		var relevance float64
		if err := rows.Scan(&item.ID, &item.Type, &item.Label, &item.Value, &item.ParentID, &item.AuthorID, &item.CommunityID,
			&item.MacroGroupID, &item.Title, &item.Body, &item.Description, &item.SourcePermalink, &sourceSensitive,
			&administrativeSensitive, &item.UpdatedAt, &item.Metrics, &item.Distinctiveness, &item.AffinityConfidence, &relevance); err != nil {
			writeRevisionError(w, err)
			return
		}
		item.Sensitive = sourceSensitive || administrativeSensitive
		if sourceSensitive {
			item.SensitivitySources = append(item.SensitivitySources, "source")
		}
		if administrativeSensitive {
			item.SensitivitySources = append(item.SensitivitySources, "administrative")
		}
		if !item.Sensitive && query != "" {
			text := strings.TrimSpace(strings.Join([]string{item.Title, item.Description, item.Body}, " "))
			if len(text) > 240 {
				text = text[:240] + "…"
			}
			item.Snippet = text
		}
		out.Items = append(out.Items, item)
	}
	if err := rows.Err(); err != nil {
		writeRevisionError(w, err)
		return
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		next, _ := encodeCatalogCursor(catalogCursor{Revision: revisionID, Catalog: catalog.Int64, Fingerprint: catalogFingerprint(r.URL.Query()), Offset: token.Offset + limit})
		out.NextCursor = next
	}
	out.ReturnedCount = len(out.Items)
	writeJSON(w, out)
}

type catalogFacet struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

func (h *RevisionHandler) CatalogFacets(w http.ResponseWriter, r *http.Request) {
	revisionID, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	catalog, err := h.catalogID(r.Context(), revisionID)
	if err != nil || !catalog.Valid {
		writeRevisionError(w, errors.New("published spatial catalog unavailable"))
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	kind := strings.TrimSpace(r.URL.Query().Get("type"))
	sensitive := strings.TrimSpace(r.URL.Query().Get("sensitive"))
	if kind != "" && !catalogTypes[kind] {
		writeRevisionError(w, errors.New("type must be subreddit, user, post, or comment"))
		return
	}
	if sensitive != "" && sensitive != "true" && sensitive != "false" {
		writeRevisionError(w, errors.New("sensitive must be true or false"))
		return
	}
	communityID := strings.TrimSpace(r.URL.Query().Get("community_id"))
	macroGroupID := strings.TrimSpace(r.URL.Query().Get("macro_group_id"))
	parentID := strings.TrimSpace(r.URL.Query().Get("parent_id"))
	authorID := strings.TrimSpace(r.URL.Query().Get("author_id"))
	relation := strings.TrimSpace(r.URL.Query().Get("relation"))
	rows, err := h.db.QueryContext(r.Context(), `SELECT e.type,count(*) FROM spatial_catalog_entities e
LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id
LEFT JOIN graph_revision_communities c ON c.revision_id=$2 AND c.community_id=e.community_id
WHERE e.catalog_id=$1 AND NOT EXISTS (SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)
AND ($3='' OR d.search_vector @@ plainto_tsquery('simple',$3) OR lower(e.label) LIKE '%'||lower($3)||'%')
AND ($4='' OR e.type=$4) AND ($5='' OR e.community_id=$5) AND ($6='' OR c.macro_group_id=$6)
AND ($7='' OR e.parent_id=$7) AND ($8='' OR e.author_id=$8)
AND ($9='' OR EXISTS (SELECT 1 FROM spatial_catalog_links l WHERE l.catalog_id=e.catalog_id AND l.relation=$9 AND (l.source=e.id OR l.target=e.id)))
AND ($10='' OR (d.source_sensitive OR d.administrative_sensitive)=($10='true'))
GROUP BY e.type ORDER BY e.type`, catalog.Int64, revisionID, query, kind, communityID, macroGroupID, parentID, authorID, relation, sensitive)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	facets := []catalogFacet{}
	var total int64
	for rows.Next() {
		var f catalogFacet
		if err := rows.Scan(&f.Type, &f.Count); err != nil {
			writeRevisionError(w, err)
			return
		}
		total += f.Count
		facets = append(facets, f)
	}
	writeJSON(w, map[string]any{"revision_id": revisionID, "spatial_catalog_id": catalog.Int64, "exact_total": total, "types": facets,
		"applied_filters": map[string]string{"q": query, "type": kind, "community_id": communityID, "macro_group_id": macroGroupID, "parent_id": parentID, "author_id": authorID, "relation": relation, "sensitive": sensitive}, "partial": false})
}

type treeChild struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Label      string          `json:"label"`
	Value      int64           `json:"value"`
	AuthorID   string          `json:"author_id,omitempty"`
	ChildCount int64           `json:"child_count"`
	Sensitive  bool            `json:"sensitive"`
	Metrics    json.RawMessage `json:"metrics"`
}

// CatalogTree returns exactly one containment level. Users are emitted as
// cross-links and are never represented as false containment children.
func (h *RevisionHandler) CatalogTree(w http.ResponseWriter, r *http.Request) {
	rootID, err := catalogEntityID(mux.Vars(r)["id"])
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	revisionID, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	catalog, err := h.catalogID(r.Context(), revisionID)
	if err != nil || !catalog.Valid {
		writeRevisionError(w, errors.New("published spatial catalog unavailable"))
		return
	}
	suppressed, err := isSuppressed(h.db, r, rootID)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if suppressed {
		http.Error(w, "gone", http.StatusGone)
		return
	}
	var rootType, rootLabel string
	err = h.db.QueryRowContext(r.Context(), `SELECT type,label FROM spatial_catalog_entities WHERE catalog_id=$1 AND id=$2`, catalog.Int64, rootID).Scan(&rootType, &rootLabel)
	if errors.Is(err, sql.ErrNoRows) {
		err = h.db.QueryRowContext(r.Context(), `SELECT 'community',COALESCE(display_name,label) FROM graph_revision_communities WHERE revision_id=$1 AND community_id=$2`, revisionID, rootID).Scan(&rootType, &rootLabel)
	}
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "entity not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	limit := namedQueryLimit(r, "limit", 50, 200)
	boundValues := r.URL.Query()
	boundValues = cloneValues(boundValues)
	boundValues.Set("tree_id", rootID)
	token, err := decodeCatalogCursor(r.URL.Query().Get("cursor"), revisionID, catalog.Int64, boundValues)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	condition := "e.parent_id=$2"
	if rootType == "community" {
		condition = "e.community_id=$2 AND e.type='subreddit'"
	}
	rows, err := h.db.QueryContext(r.Context(), fmt.Sprintf(`SELECT e.id,e.type,e.label,e.value,COALESCE(e.author_id,''),
 (SELECT count(*) FROM spatial_catalog_entities child WHERE child.catalog_id=e.catalog_id AND child.parent_id=e.id
   AND NOT EXISTS (SELECT 1 FROM public_entity_suppressions suppressed_child WHERE suppressed_child.entity_id=child.id AND suppressed_child.restored_at IS NULL)),
 COALESCE(d.source_sensitive OR d.administrative_sensitive,false),e.metrics
FROM spatial_catalog_entities e LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id
WHERE e.catalog_id=$1 AND %s
 AND NOT EXISTS (SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)
ORDER BY e.value DESC,e.id OFFSET $3 LIMIT $4`, condition), catalog.Int64, rootID, token.Offset, limit+1)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	children := []treeChild{}
	authors := map[string]bool{}
	for rows.Next() {
		var child treeChild
		if err := rows.Scan(&child.ID, &child.Type, &child.Label, &child.Value, &child.AuthorID, &child.ChildCount, &child.Sensitive, &child.Metrics); err != nil {
			writeRevisionError(w, err)
			return
		}
		if child.AuthorID != "" {
			authors[child.AuthorID] = true
		}
		children = append(children, child)
	}
	next := ""
	if len(children) > limit {
		children = children[:limit]
		next, _ = encodeCatalogCursor(catalogCursor{Revision: revisionID, Catalog: catalog.Int64, Fingerprint: catalogFingerprint(boundValues), Offset: token.Offset + limit})
	}
	authorLinks := make([]string, 0, len(authors))
	for id := range authors {
		authorLinks = append(authorLinks, id)
	}
	sort.Strings(authorLinks)
	writeJSON(w, map[string]any{"revision_id": revisionID, "spatial_catalog_id": catalog.Int64, "root": map[string]any{"id": rootID, "type": rootType, "label": rootLabel},
		"children": children, "returned_count": len(children), "cross_linked_authors": authorLinks, "next_cursor": next, "partial": false, "stale": false})
}

func cloneValues(values url.Values) url.Values {
	clone := url.Values{}
	for key, items := range values {
		for _, item := range items {
			clone.Add(key, item)
		}
	}
	return clone
}

type macroGroup struct {
	ID                     string          `json:"id"`
	DisplayName            string          `json:"display_name"`
	EvidenceLabel          string          `json:"evidence_label"`
	PaletteRole            int             `json:"palette_role"`
	PrimaryColor           string          `json:"primary_color"`
	MemberCount            int64           `json:"member_count"`
	RepresentativeClusters json.RawMessage `json:"representative_clusters"`
	EvidenceMetrics        json.RawMessage `json:"evidence_metrics"`
}

type macroGroupLink struct {
	Source           string  `json:"source"`
	Target           string  `json:"target"`
	Affinity         float64 `json:"affinity"`
	ObservedEvidence float64 `json:"observed_evidence"`
}

func (h *RevisionHandler) MacroGroups(w http.ResponseWriter, r *http.Request) {
	revisionID, err := h.revision(r)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT macro_group_id,display_name,evidence_label,palette_role,primary_color,member_count,
representative_clusters,evidence_metrics FROM graph_revision_macro_groups g WHERE revision_id=$1
AND NOT EXISTS (SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=g.macro_group_id AND s.restored_at IS NULL)
ORDER BY palette_role,macro_group_id`, revisionID)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	groups := []macroGroup{}
	for rows.Next() {
		var group macroGroup
		if err := rows.Scan(&group.ID, &group.DisplayName, &group.EvidenceLabel, &group.PaletteRole, &group.PrimaryColor, &group.MemberCount, &group.RepresentativeClusters, &group.EvidenceMetrics); err != nil {
			writeRevisionError(w, err)
			return
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		writeRevisionError(w, err)
		return
	}
	linkRows, err := h.db.QueryContext(r.Context(), `SELECT source_macro_group_id,target_macro_group_id,affinity,observed_evidence
FROM graph_revision_macro_group_links l WHERE revision_id=$1
AND NOT EXISTS (SELECT 1 FROM public_entity_suppressions s WHERE s.restored_at IS NULL AND s.entity_id IN (l.source_macro_group_id,l.target_macro_group_id))
ORDER BY affinity DESC,source_macro_group_id,target_macro_group_id`, revisionID)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer linkRows.Close()
	links := []macroGroupLink{}
	for linkRows.Next() {
		var link macroGroupLink
		if err := linkRows.Scan(&link.Source, &link.Target, &link.Affinity, &link.ObservedEvidence); err != nil {
			writeRevisionError(w, err)
			return
		}
		links = append(links, link)
	}
	writeJSON(w, map[string]any{"revision_id": revisionID, "macro_groups": groups, "inter_group_affinity": links, "count": len(groups)})
}

func catalogEntityID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 256 {
		return "", errors.New("invalid entity id")
	}
	return raw, nil
}

func parseCatalogOffset(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errors.New("invalid offset")
	}
	return value, nil
}
