package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/utils"
)

// SubredditsLister abstracts listing with pagination for testability.
type SubredditsLister interface {
	ListSubreddits(ctx context.Context, params db.ListSubredditsParams) ([]db.Subreddit, error)
}

func GetSubreddits(q SubredditsLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Defaults
		limit := int32(100)
		offset := int32(0)

		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := utils.Atoi32(v); err == nil && n > 0 {
				limit = int32(n)
			}
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := utils.Atoi32(v); err == nil && n >= 0 {
				offset = int32(n)
			}
		}

		subs, err := q.ListSubreddits(r.Context(), db.ListSubredditsParams{Limit: limit, Offset: offset})
		if err != nil {
			http.Error(w, "Failed to fetch subreddits", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(subs); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}
}
