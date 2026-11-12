package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

type queueItem struct {
	ID            int32  `json:"id"`
	SubredditID   int32  `json:"subreddit_id"`
	SubredditName string `json:"subreddit_name"`
	Status        string `json:"status"`
	Priority      int32  `json:"priority"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func GetCrawlStatus(q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const qstr = `SELECT cj.id, cj.subreddit_id, s.name AS subreddit_name, cj.status, cj.priority, cj.created_at::text, cj.updated_at::text
                       FROM crawl_jobs cj JOIN subreddits s ON s.id = cj.subreddit_id
                       WHERE cj.status IN ('queued','crawling')
                       ORDER BY cj.priority DESC, cj.created_at ASC`
		rows, err := q.DB().QueryContext(r.Context(), qstr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var out []queueItem
		for rows.Next() {
			var it queueItem
			if err := rows.Scan(&it.ID, &it.SubredditID, &it.SubredditName, &it.Status, &it.Priority, &it.CreatedAt, &it.UpdatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out = append(out, it)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Summary counts across all jobs
		var qd, cr, su, fa sql.NullInt64
		const csql = `SELECT
                        COALESCE(SUM(CASE WHEN status='queued' THEN 1 ELSE 0 END),0),
                        COALESCE(SUM(CASE WHEN status='crawling' THEN 1 ELSE 0 END),0),
                        COALESCE(SUM(CASE WHEN status='success' THEN 1 ELSE 0 END),0),
                        COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0)
                    FROM crawl_jobs`
		_ = q.DB().QueryRowContext(r.Context(), csql).Scan(&qd, &cr, &su, &fa)

		counts := map[string]int64{
			"queued":   qd.Int64,
			"crawling": cr.Int64,
			"success":  su.Int64,
			"failed":   fa.Int64,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"queue": out, "counts": counts})
	}
}
