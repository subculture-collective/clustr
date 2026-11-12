package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/utils"
)

func GetCrawlJobs(q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 10
		offset := 0

		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if l, err := utils.Atoi32(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if o, err := utils.Atoi32(offsetStr); err == nil && o >= 0 {
				offset = o
			}
		}

		jobs, err := q.ListCrawlJobs(r.Context(), db.ListCrawlJobsParams{
			Column1: int32(limit),
			Column2: int32(offset),
		})
		if err != nil {
			http.Error(w, "Failed to fetch crawl jobs", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobs)
	}
}
