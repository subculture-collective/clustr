package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/utils"
)

func GetPosts(q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subredditIDStr := r.URL.Query().Get("subreddit_id")
		if subredditIDStr == "" {
			http.Error(w, "subreddit_id is required", http.StatusBadRequest)
			return
		}

		subredditID, err := strconv.ParseInt(subredditIDStr, 10, 32)
		if err != nil {
			http.Error(w, "Invalid subreddit_id", http.StatusBadRequest)
			return
		}

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

		posts, err := q.ListPostsBySubreddit(r.Context(), db.ListPostsBySubredditParams{
			SubredditID: int32(subredditID),
			Limit:       int32(limit),
			Offset:      int32(offset),
		})
		if err != nil {
			http.Error(w, "Failed to fetch posts", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(posts)
	}
}
