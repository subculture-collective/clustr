package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

func GetUsers(q *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := db.ListUsersParams{
			Limit:  100,
			Offset: 0,
		}

		users, err := q.ListUsers(r.Context(), params)
		if err != nil {
			http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(users); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}
}
