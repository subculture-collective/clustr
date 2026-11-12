package crawler

import (
	"database/sql"

	"github.com/subculture-collective/clustr/backend/internal/db"
)

// ToUpsertPostParams converts a Post into UpsertPostParams.
func ToUpsertPostParams(p Post, subredditID int32, authorID int32) db.UpsertPostParams {
	return db.UpsertPostParams{
		ID:          p.ID,
		SubredditID: subredditID,
		AuthorID:    authorID,
		Title:       sql.NullString{String: p.Title, Valid: p.Title != ""},
		Selftext:    sql.NullString{String: p.Selftext, Valid: p.Selftext != ""},
		Permalink:   sql.NullString{String: p.Permalink, Valid: p.Permalink != ""},
		Score:       sql.NullInt32{Int32: int32(p.Score), Valid: true},
		Flair:       sql.NullString{String: p.Flair, Valid: p.Flair != ""},
		Url:         sql.NullString{String: p.URL, Valid: p.URL != ""},
		IsSelf:      sql.NullBool{Bool: p.IsSelf, Valid: true},
		CreatedAt:   sql.NullTime{Time: p.CreatedAt, Valid: !p.CreatedAt.IsZero()},
	}
}

// ToUpsertCommentParams converts a Comment into UpsertCommentParams.
func ToUpsertCommentParams(c Comment, postID string, subredditID int32, authorID int32) db.UpsertCommentParams {
	return db.UpsertCommentParams{
		ID:          c.ID,
		PostID:      postID,
		AuthorID:    authorID,
		SubredditID: subredditID,
		Body:        sql.NullString{String: c.Body, Valid: c.Body != ""},
		CreatedAt:   sql.NullTime{Time: c.CreatedAt, Valid: !c.CreatedAt.IsZero()},
		ParentID:    sql.NullString{String: c.ParentID, Valid: c.ParentID != ""},
		Score:       sql.NullInt32{Int32: int32(c.Score), Valid: true},
		Depth:       sql.NullInt32{Int32: int32(c.Depth), Valid: true},
	}
}
