package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

func GetComments(q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postID := r.URL.Query().Get("post_id")
		if postID == "" {
			http.Error(w, "post_id is required", http.StatusBadRequest)
			return
		}

		comments, err := q.ListCommentsByPost(r.Context(), postID)
		if err != nil {
			http.Error(w, "Failed to fetch comments", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comments)
	}
}
