package handlers

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

type CatalogPageHandler struct {
	db      db.DBTX
	baseURL string
}

func NewCatalogPageHandler(database db.DBTX) *CatalogPageHandler {
	base := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/")
	if base == "" {
		base = "https://clustr.subcult.tv"
	}
	return &CatalogPageHandler{db: database, baseURL: base}
}

type catalogPageData struct {
	Title, Kind, ID, Summary, Canonical, Robots, Updated, ProvenanceJSON, SourceURL string
	EntityJSON                                                                      template.JS
	Sensitive                                                                       bool
}

var catalogPageTemplate = template.Must(template.New("catalog-page").Parse(`<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}} · Clustr Catalog</title><meta name="description" content="{{.Summary}}"><meta name="robots" content="{{.Robots}}"><link rel="canonical" href="{{.Canonical}}"><link rel="stylesheet" href="/assets/app.css"><script type="application/ld+json">{{.EntityJSON}}</script><script type="module" src="/assets/app.js"></script></head><body><div id="root"><main id="catalog-document"><p>CLUSTR PUBLIC CATALOG</p><h1>{{.Title}}</h1><p>{{.Kind}} · {{.ID}}</p>{{if .Sensitive}}<section aria-label="Sensitive content warning"><h2>Sensitive content</h2><p>Full text is collapsed in the interactive Catalog. Non-sensitive metadata remains visible.</p></section>{{else}}<p>{{.Summary}}</p>{{end}}<dl><dt>Published record</dt><dd>{{.Updated}}</dd><dt>Provenance</dt><dd>{{.ProvenanceJSON}}</dd></dl>{{if .SourceURL}}<p><a href="{{.SourceURL}}" rel="ugc nofollow noopener noreferrer">View public source</a></p>{{end}}<p><a href="/?view=catalog&amp;catalog_selected={{.ID}}">Open interactive Catalog</a></p></main></div></body></html>`))

func (h *CatalogPageHandler) Entity(w http.ResponseWriter, r *http.Request) {
	kind, id := strings.TrimSuffix(mux.Vars(r)["type"], "s"), mux.Vars(r)["id"]
	if (!catalogTypes[kind] && kind != "cluster") || id == "" {
		http.NotFound(w, r)
		return
	}
	var revisionID, catalogID int64
	var snapshotErr error
	if rawRevision := strings.TrimSpace(r.URL.Query().Get("revision")); rawRevision != "" {
		requestedRevision, parseErr := strconv.ParseInt(rawRevision, 10, 64)
		if parseErr != nil || requestedRevision < 1 {
			http.Error(w, "invalid revision", http.StatusBadRequest)
			return
		}
		snapshotErr = h.db.QueryRowContext(r.Context(), `SELECT id,spatial_catalog_id FROM graph_revisions WHERE id=$1 AND status='published' AND spatial_catalog_id IS NOT NULL`, requestedRevision).Scan(&revisionID, &catalogID)
	} else {
		snapshotErr = h.db.QueryRowContext(r.Context(), `SELECT r.id,r.spatial_catalog_id FROM graph_revision_current current JOIN graph_revisions r ON r.id=current.revision_id WHERE current.singleton AND r.status='published'`).Scan(&revisionID, &catalogID)
	}
	if snapshotErr != nil {
		http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	var suppressed bool
	if err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM public_entity_suppressions WHERE entity_id=$1 AND restored_at IS NULL)`, id).Scan(&suppressed); err != nil {
		http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	if suppressed {
		http.Error(w, "gone", http.StatusGone)
		return
	}
	var label, title, body, description, source, updated string
	var sourceSensitive, adminSensitive bool
	var provenance []byte
	var err error
	if kind == "cluster" {
		err = h.db.QueryRowContext(r.Context(), `SELECT label,COALESCE(display_name,label),'',COALESCE(evidence_label,label),'',false,false,r.published_at::text,evidence_metrics FROM graph_revision_communities c JOIN graph_revisions r ON r.id=c.revision_id WHERE c.revision_id=$1 AND c.community_id=$2`, revisionID, id).Scan(&label, &title, &body, &description, &source, &sourceSensitive, &adminSensitive, &updated, &provenance)
	} else {
		err = h.db.QueryRowContext(r.Context(), `SELECT e.label,COALESCE(d.title,''),COALESCE(d.body,''),COALESCE(d.description,''),COALESCE(d.source_permalink,''),COALESCE(d.source_sensitive,false),COALESCE(d.administrative_sensitive,false),e.source_updated_at::text,COALESCE(d.provenance,'{}'::jsonb) FROM spatial_catalog_entities e LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id WHERE e.catalog_id=$1 AND e.id=$2 AND e.type=$3`, catalogID, id, kind).Scan(&label, &title, &body, &description, &source, &sourceSensitive, &adminSensitive, &updated, &provenance)
	}
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	sensitive := sourceSensitive || adminSensitive
	if sensitive {
		title, body, description = "Sensitive "+kind+" record", "", ""
	}
	summary := strings.Join(strings.Fields(strings.TrimSpace(strings.Join([]string{title, description, body}, " "))), " ")
	if sensitive {
		summary = "Sensitive public record; metadata is available in the Clustr Catalog."
	}
	if len(summary) > 320 {
		summary = summary[:320] + "…"
	}
	if summary == "" {
		summary = label + " in the Clustr public relationship catalog."
	}
	canonical := fmt.Sprintf("%s/catalog/%ss/%s", h.baseURL, kind, url.PathEscape(id))
	robots := "index,follow"
	if sensitive || r.URL.Query().Has("revision") {
		robots = "noindex,follow"
	}
	entityLD, _ := json.Marshal(map[string]any{"@context": "https://schema.org", "@type": "CreativeWork", "name": firstNonempty(title, label), "identifier": id, "url": canonical, "dateModified": updated, "isAccessibleForFree": true})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Clustr-Revision", strconv.FormatInt(revisionID, 10))
	_ = catalogPageTemplate.Execute(w, catalogPageData{Title: firstNonempty(title, label), Kind: kind, ID: id, Summary: summary, Canonical: canonical, Robots: robots, Updated: updated, ProvenanceJSON: string(provenance), EntityJSON: template.JS(entityLD), SourceURL: source, Sensitive: sensitive})
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "Clustr Catalog record"
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}
type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}
type sitemapIndex struct {
	XMLName xml.Name     `xml:"sitemapindex"`
	Xmlns   string       `xml:"xmlns,attr"`
	Maps    []sitemapMap `xml:"sitemap"`
}
type sitemapMap struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func (h *CatalogPageHandler) SitemapIndex(w http.ResponseWriter, r *http.Request) {
	var revisionID, catalogID int64
	var publishedDate string
	if err := h.db.QueryRowContext(r.Context(), `SELECT r.id,r.spatial_catalog_id,to_char(r.published_at,'YYYY-MM-DD') FROM graph_revision_current current JOIN graph_revisions r ON r.id=current.revision_id WHERE current.singleton AND r.status='published'`).Scan(&revisionID, &catalogID, &publishedDate); err != nil {
		http.Error(w, "catalog unavailable", 503)
		return
	}
	tiers := []int{1}
	if envTrue("SITEMAP_TIER2_ENABLED") {
		tiers = append(tiers, 2)
	}
	if envTrue("SITEMAP_TIER3_ENABLED") {
		tiers = append(tiers, 3)
	}
	maps := []sitemapMap{}
	for _, tier := range tiers {
		var count int64
		query, args := sitemapCountQuery(tier, catalogID)
		if err := h.db.QueryRowContext(r.Context(), query, args...).Scan(&count); err != nil {
			http.Error(w, "sitemap unavailable", 503)
			return
		}
		for shard := int64(0); shard*50000 < count; shard++ {
			maps = append(maps, sitemapMap{Loc: fmt.Sprintf("%s/sitemaps/%d/%d.xml", h.baseURL, tier, shard), LastMod: publishedDate})
		}
	}
	writeXML(w, sitemapIndex{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", Maps: maps})
}

func (h *CatalogPageHandler) SitemapShard(w http.ResponseWriter, r *http.Request) {
	tier, _ := strconv.Atoi(mux.Vars(r)["tier"])
	shard, _ := strconv.Atoi(mux.Vars(r)["shard"])
	if tier < 1 || tier > 3 || shard < 0 || (tier == 2 && !envTrue("SITEMAP_TIER2_ENABLED")) || (tier == 3 && !envTrue("SITEMAP_TIER3_ENABLED")) {
		http.NotFound(w, r)
		return
	}
	var revisionID, catalogID int64
	if err := h.db.QueryRowContext(r.Context(), `SELECT r.id,r.spatial_catalog_id FROM graph_revision_current current JOIN graph_revisions r ON r.id=current.revision_id WHERE current.singleton AND r.status='published'`).Scan(&revisionID, &catalogID); err != nil {
		http.Error(w, "catalog unavailable", 503)
		return
	}
	query, args := sitemapRowsQuery(tier, catalogID, shard*50000)
	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, "sitemap unavailable", 503)
		return
	}
	defer rows.Close()
	urls := []sitemapURL{}
	for rows.Next() {
		var id, kind, updated string
		if err := rows.Scan(&id, &kind, &updated); err != nil {
			http.Error(w, "sitemap unavailable", 503)
			return
		}
		urls = append(urls, sitemapURL{Loc: fmt.Sprintf("%s/catalog/%ss/%s", h.baseURL, kind, url.PathEscape(id)), LastMod: updated})
	}
	writeXML(w, sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls})
}

func sitemapCountQuery(tier int, catalogID int64) (string, []any) {
	base := `SELECT count(*) FROM spatial_catalog_entities e LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id WHERE e.catalog_id=$1 AND NOT COALESCE(d.source_sensitive OR d.administrative_sensitive,false) AND NOT EXISTS(SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)`
	if tier == 1 {
		return `SELECT (SELECT count(*) FROM graph_revision_communities c JOIN graph_revision_current current ON current.revision_id=c.revision_id WHERE current.singleton AND NOT EXISTS(SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=c.community_id AND s.restored_at IS NULL)) + (` + base + ` AND e.type='subreddit')`, []any{catalogID}
	}
	if tier == 2 {
		return `SELECT LEAST(count(*),$2) FROM spatial_catalog_entities e LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id WHERE e.catalog_id=$1 AND e.type IN ('user','post') AND NOT COALESCE(d.source_sensitive OR d.administrative_sensitive,false) AND NOT EXISTS(SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)`, []any{catalogID, sitemapTier2Limit()}
	}
	return `SELECT
(SELECT count(*) FROM spatial_catalog_entities e LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id WHERE e.catalog_id=$1 AND e.type='comment' AND NOT COALESCE(d.source_sensitive OR d.administrative_sensitive,false) AND NOT EXISTS(SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)) +
GREATEST((SELECT count(*) FROM spatial_catalog_entities e LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id WHERE e.catalog_id=$1 AND e.type IN ('user','post') AND NOT COALESCE(d.source_sensitive OR d.administrative_sensitive,false) AND NOT EXISTS(SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL))-$2,0)`, []any{catalogID, sitemapTier2Limit()}
}
func sitemapRowsQuery(tier int, catalogID int64, offset int) (string, []any) {
	base := `SELECT e.id,e.type,to_char(e.source_updated_at,'YYYY-MM-DD') updated,e.value FROM spatial_catalog_entities e LEFT JOIN spatial_catalog_documents d ON d.catalog_id=e.catalog_id AND d.entity_id=e.id WHERE e.catalog_id=$1 AND NOT COALESCE(d.source_sensitive OR d.administrative_sensitive,false) AND NOT EXISTS(SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=e.id AND s.restored_at IS NULL)`
	if tier == 1 {
		return `SELECT id,type,updated FROM (` + base + ` AND e.type='subreddit' UNION ALL SELECT c.community_id,'cluster',to_char(r.published_at,'YYYY-MM-DD'),c.size FROM graph_revision_communities c JOIN graph_revision_current current ON current.revision_id=c.revision_id JOIN graph_revisions r ON r.id=c.revision_id WHERE current.singleton AND NOT EXISTS(SELECT 1 FROM public_entity_suppressions s WHERE s.entity_id=c.community_id AND s.restored_at IS NULL)) tier ORDER BY value DESC,id OFFSET $2 LIMIT 50000`, []any{catalogID, offset}
	}
	if tier == 2 {
		return `WITH eligible AS (` + base + ` AND e.type IN ('user','post') ORDER BY e.value DESC,e.id LIMIT $2)
SELECT id,type,updated FROM eligible ORDER BY value DESC,id OFFSET $3 LIMIT 50000`, []any{catalogID, sitemapTier2Limit(), offset}
	}
	return `WITH ranked AS (SELECT candidate.*,row_number() OVER(ORDER BY candidate.value DESC,candidate.id) activity_rank FROM (` + base + ` AND e.type IN ('user','post')) candidate), eligible AS (
SELECT id,type,updated,value FROM ranked WHERE activity_rank>$2
UNION ALL
SELECT id,type,updated,value FROM (` + base + ` AND e.type='comment') comments)
SELECT id,type,updated FROM eligible ORDER BY value DESC,id OFFSET $3 LIMIT 50000`, []any{catalogID, sitemapTier2Limit(), offset}
}

func sitemapTier2Limit() int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("SITEMAP_TIER2_ENTITY_LIMIT")), 10, 64)
	if err != nil || value < 1 {
		return 250000
	}
	return value
}
func envTrue(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes"
}
func writeXML(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(value)
}
