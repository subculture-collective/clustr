package crawler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/logger"
	"github.com/onnwee/reddit-cluster-map/backend/internal/metrics"
	"github.com/onnwee/reddit-cluster-map/backend/internal/tracing"
	"github.com/onnwee/reddit-cluster-map/backend/internal/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	MaxPostsPerSubreddit = utils.GetEnvAsInt("MAX_POSTS_PER_SUB", 50)
	MaxCommentsPerPost   = utils.GetEnvAsInt("MAX_COMMENTS_PER_POST", 100)
	MaxCommentDepth      = utils.GetEnvAsInt("MAX_COMMENT_DEPTH", 3)
	DefaultSubs          = utils.GetEnvAsSlice("DEFAULT_SUBREDDITS", []string{"AskReddit", "worldnews", "technology", "funny", "gaming"}, ",")
)

func handleJob(ctx context.Context, q *db.Queries, job db.CrawlJob) error {
	_, err := executeJob(ctx, q, job)
	return err
}

func executeJob(ctx context.Context, q *db.Queries, job db.CrawlJob) (AttemptResult, error) {
	result := AttemptResult{Outcome: OutcomeComplete}
	ctx, span := tracing.StartSpan(ctx, "crawler.handleJob")
	defer span.End()

	span.SetAttributes(
		attribute.Int("job_id", int(job.ID)),
		attribute.Int("subreddit_id", int(job.SubredditID)),
	)

	startTime := time.Now()
	var jobStatus string
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.CrawlerJobDuration.WithLabelValues(jobStatus).Observe(duration)
		metrics.CrawlerJobsTotal.WithLabelValues(jobStatus).Inc()
		span.SetAttributes(
			attribute.String("job_status", jobStatus),
			attribute.Float64("duration_seconds", duration),
		)
	}()

	logger.InfoContext(ctx, "Starting crawl job", "job_id", job.ID)

	// Update job status to crawling
	if err := q.MarkCrawlJobStarted(ctx, job.ID); err != nil {
		logger.WarnContext(ctx, "Failed to update job status to crawling", "error", err, "job_id", job.ID)
		span.RecordError(err)
		return result, err
	}

	// Get subreddit name from ID
	subreddit, err := q.GetSubredditByID(ctx, job.SubredditID)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to get subreddit", "error", err, "subreddit_id", job.SubredditID)
		// Update job status to failed
		_ = q.MarkCrawlJobFailed(ctx, job.ID)
		jobStatus = "failed"
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get subreddit")
		return result, err
	}

	span.SetAttributes(attribute.String("subreddit", subreddit.Name))
	logger.InfoContext(ctx, "Crawling subreddit", "subreddit", subreddit.Name)

	info, posts, err := CrawlSubredditContext(ctx, subreddit.Name)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to crawl subreddit", "error", err, "subreddit", subreddit.Name)
		// Update job status to failed
		_ = q.MarkCrawlJobFailed(ctx, job.ID)
		jobStatus = "failed"
		span.RecordError(err)
		span.SetStatus(codes.Error, "crawl failed")
		return result, err
	}

	logger.InfoContext(ctx, "Crawled subreddit successfully",
		"subreddit", subreddit.Name,
		"posts", len(posts),
		"subscribers", info.Subscribers,
	)
	span.SetAttributes(
		attribute.Int("posts_count", len(posts)),
		attribute.Int("subscribers", info.Subscribers),
	)

	// Update subreddit info
	_, err = q.UpsertSubreddit(ctx, db.UpsertSubredditParams{
		Name:        subreddit.Name,
		Title:       sql.NullString{String: info.Title, Valid: info.Title != ""},
		Description: sql.NullString{String: info.Description, Valid: info.Description != ""},
		Subscribers: sql.NullInt32{Int32: int32(info.Subscribers), Valid: info.Subscribers >= 0},
	})
	if err != nil {
		logger.WarnContext(ctx, "Failed to upsert subreddit", "error", err, "subreddit", subreddit.Name)
		// Update job status to failed
		_ = q.MarkCrawlJobFailed(ctx, job.ID)
		jobStatus = "failed"
		span.RecordError(err)
		span.SetStatus(codes.Error, "upsert failed")
		return result, err
	}
	logger.DebugContext(ctx, "Updated subreddit info", "subreddit", subreddit.Name)

	if len(posts) > MaxPostsPerSubreddit {
		logger.DebugContext(ctx, "Limiting posts", "from", len(posts), "to", MaxPostsPerSubreddit)
		posts = posts[:MaxPostsPerSubreddit]
	}

	insertedPosts, err := crawlAndStorePosts(ctx, q, job.SubredditID, posts)
	if err != nil {
		log.Printf("⚠️ Failed to crawl and store posts: %v", err)
		// Update job status to failed
		_ = q.MarkCrawlJobFailed(ctx, job.ID)
		jobStatus = "failed"
		return result, err
	}
	log.Printf("✅ Stored %d posts", len(insertedPosts))
	result.Posts = len(insertedPosts)
	result.ItemFailures += len(posts) - len(insertedPosts)

	comments, commentFailures, err := crawlAndStoreComments(ctx, q, job.SubredditID, posts, utils.GetEnvAsInt("MAX_COMMENT_DEPTH", 5), insertedPosts)
	result.Comments = comments
	result.ItemFailures += commentFailures
	if err != nil {
		log.Printf("⚠️ Failed to crawl and store comments: %v", err)
		// Update job status to failed
		_ = q.MarkCrawlJobFailed(ctx, job.ID)
		jobStatus = "failed"
		return result, err
	}
	if result.ItemFailures > 0 {
		result.Outcome = OutcomePartial
		result.ErrorClass = "optional_item_failures"
		result.ErrorMessage = fmt.Sprintf("%d post/comment items were unavailable or rejected", result.ItemFailures)
	}

	enqueueLinkedSubreddits(ctx, q, posts)

	duration := time.Since(startTime)
	log.Printf("🎉 Completed crawl job #%d for r/%s in %v", job.ID, subreddit.Name, duration)

	// Update job status to success
	if err := q.MarkCrawlJobSuccess(ctx, job.ID); err != nil {
		log.Printf("⚠️ Failed to update job status to success: %v", err)
		jobStatus = "failed"
		return result, err
	}

	jobStatus = "success"
	return result, nil
}

func crawlAndStorePosts(ctx context.Context, q *db.Queries, subredditID int32, posts []Post) (map[string]bool, error) {
	insertedPosts := make(map[string]bool)
	skippedPosts := 0
	insertedCount := 0

	for _, post := range posts {
		if post.Author == "" || post.Author == "[deleted]" {
			skippedPosts++
			continue
		}

		// Get or create user
		if err := q.UpsertUser(ctx, post.Author); err != nil {
			log.Printf("⚠️ Failed to upsert user %s: %v", post.Author, err)
			skippedPosts++
			continue
		}

		// Get user ID
		user, err := q.GetUser(ctx, post.Author)
		if err != nil {
			log.Printf("⚠️ Failed to get user %s: %v", post.Author, err)
			skippedPosts++
			continue
		}
		params := ToUpsertPostParams(post, subredditID, user.ID)
		if err := q.UpsertPost(ctx, params); err != nil {
			log.Printf("⚠️ Failed to upsert post (ID=%s, Author=%s): %v", post.ID, post.Author, err)
			skippedPosts++
		} else {
			removed := post.RemovedByCategory != "" || post.Selftext == "[removed]"
			deleted := post.Selftext == "[deleted]"
			if _, flagErr := q.DB().ExecContext(ctx, `UPDATE posts SET source_sensitive=$2,source_removed=$3,source_deleted=$4 WHERE id=$1`, post.ID, post.Over18, removed, deleted); flagErr != nil {
				return insertedPosts, fmt.Errorf("persist post source flags: %w", flagErr)
			}
			if err := persistPostReferences(ctx, q, subredditID, post); err != nil {
				return insertedPosts, err
			}
			insertedPosts[post.ID] = true
			insertedCount++
			metrics.CrawlerPostsProcessed.Inc()
		}
	}

	log.Printf("📝 Posts: %d inserted, %d skipped", insertedCount, skippedPosts)
	return insertedPosts, nil
}

func crawlAndStoreComments(
	ctx context.Context,
	q *db.Queries,
	subredditID int32,
	posts []Post,
	maxDepth int,
	insertedPosts map[string]bool,
) (int, int, error) {
	authorSet := make(map[string]bool)
	totalComments := 0
	totalSkipped := 0
	threadFailures := 0

	for _, post := range posts {
		insertedThisPost := 0
		skippedThisPost := 0

		postID := utils.ExtractPostID(post.Permalink)

		// Skip if post wasn't inserted
		if !insertedPosts[postID] {
			log.Printf("⚠️ Skipping comments for post %s — post not found", postID)
			continue
		}

		comments, err := CrawlCommentsContext(ctx, postID)
		if err != nil {
			log.Printf("⚠️ Failed to fetch comments for %s: %v", post.Permalink, err)
			threadFailures++
			continue
		}

		log.Printf("💬 Post: %s — %d comments", post.Title, len(comments))
		totalComments += len(comments)

		inserted := map[string]bool{}
		pending := map[string]db.UpsertCommentParams{}

		// First pass
		for _, c := range comments {
			if !utils.IsValidAuthor(c.Author) || c.Depth > maxDepth {
				skippedThisPost++
				continue
			}

			// Get or create user
			if err := q.UpsertUser(ctx, c.Author); err != nil {
				log.Printf("⚠️ Failed to upsert user %s: %v", c.Author, err)
				skippedThisPost++
				continue
			}

			// Get user ID
			user, err := q.GetUser(ctx, c.Author)
			if err != nil {
				log.Printf("⚠️ Failed to get user %s: %v", c.Author, err)
				skippedThisPost++
				continue
			}

			authorSet[c.Author] = true

			parentID := utils.StripPrefix(c.ParentID)
			params := ToUpsertCommentParams(c, postID, subredditID, user.ID)

			if parentID == "" || strings.HasPrefix(c.ParentID, "t3_") || inserted[parentID] {
				if err := q.UpsertComment(ctx, params); err == nil {
					_, _ = q.DB().ExecContext(ctx, `UPDATE comments SET source_sensitive=$2,source_removed=$3,source_deleted=$4 WHERE id=$1`, c.ID, c.Sensitive || post.Over18, c.Removed, c.Deleted)
					inserted[c.ID] = true
					insertedThisPost++
					metrics.CrawlerCommentsProcessed.Inc()
				} else {
					log.Printf("⚠️ Failed to insert comment %s: %v", c.ID, err)
					skippedThisPost++
				}
			} else {
				pending[c.ID] = params
			}
		}

		// Second pass for orphans
		for id, params := range pending {
			if inserted[utils.StripPrefix(params.ParentID.String)] {
				if err := q.UpsertComment(ctx, params); err == nil {
					comment := findComment(comments, id)
					_, _ = q.DB().ExecContext(ctx, `UPDATE comments SET source_sensitive=$2,source_removed=$3,source_deleted=$4 WHERE id=$1`, id, comment.Sensitive || post.Over18, comment.Removed, comment.Deleted)
					inserted[id] = true
					insertedThisPost++
					metrics.CrawlerCommentsProcessed.Inc()
				} else {
					log.Printf("⚠️ Second pass failed for comment %s: %v", id, err)
					skippedThisPost++
				}
			}
		}

		log.Printf("💬 Post %s: Comments inserted: %d, Skipped: %d", postID, insertedThisPost, skippedThisPost)
		totalSkipped += skippedThisPost
	}

	log.Printf("💬 Total comments processed: %d, Total skipped: %d", totalComments, totalSkipped)

	// Trigger durable discovery from authors
	var authors []string
	for author := range authorSet {
		authors = append(authors, author)
	}
	log.Printf("👥 Found %d unique authors to process", len(authors))

	FetchAndQueueUserSubredditsForAuthors(ctx, q, authors, FetchUserSubredditsConfig{
		Limit:      utils.GetEnvAsInt("USER_SUB_FETCH_LIMIT", 30),
		MaxEnqueue: utils.GetEnvAsInt("USER_SUB_ENQUEUE_MAX", 10),
		Enabled:    utils.GetEnvAsBool("FETCH_USER_SUBREDDITS", true),
	})

	return totalComments - totalSkipped, totalSkipped + threadFailures, nil
}

func persistPostReferences(ctx context.Context, q *db.Queries, sourceSubredditID int32, post Post) error {
	if len(post.CrosspostParentList) > 0 {
		parent := post.CrosspostParentList[0]
		var targetID sql.NullInt32
		_ = q.DB().QueryRowContext(ctx, `SELECT id FROM subreddits WHERE lower(name)=lower($1)`, parent.Subreddit).Scan(&targetID)
		if _, err := q.DB().ExecContext(ctx, `INSERT INTO subreddit_cross_references(source_post_id,source_subreddit_id,target_post_id,target_subreddit_name,target_subreddit_id,relation,observed_at)
VALUES($1,$2,$3,$4,$5,'crosspost',$6) ON CONFLICT DO NOTHING`, post.ID, sourceSubredditID, parent.ID, parent.Subreddit, targetID, post.CreatedAt); err != nil {
			return fmt.Errorf("persist crosspost: %w", err)
		}
	}
	for _, match := range subredditMentionRegex.FindAllStringSubmatch(post.Title+"\n"+post.Selftext, -1) {
		name := match[1]
		var targetID sql.NullInt32
		_ = q.DB().QueryRowContext(ctx, `SELECT id FROM subreddits WHERE lower(name)=lower($1)`, name).Scan(&targetID)
		if _, err := q.DB().ExecContext(ctx, `INSERT INTO subreddit_cross_references(source_post_id,source_subreddit_id,target_post_id,target_subreddit_name,target_subreddit_id,relation,observed_at)
VALUES($1,$2,$3,$4,$5,'reference',$6) ON CONFLICT DO NOTHING`, post.ID, sourceSubredditID, "subreddit:"+strings.ToLower(name), name, targetID, post.CreatedAt); err != nil {
			return fmt.Errorf("persist subreddit reference: %w", err)
		}
	}
	return nil
}

func findComment(comments []Comment, id string) Comment {
	for _, comment := range comments {
		if comment.ID == id {
			return comment
		}
	}
	return Comment{}
}

func enqueueLinkedSubreddits(ctx context.Context, q *db.Queries, posts []Post) {
	linked := extractMentionedSubreddits(posts)
	log.Printf("🔗 Found %d linked subreddits", len(linked))

	enqueuedCount := 0
	for _, sub := range linked {
		// First get or create the subreddit
		subreddit, err := q.EnsureSubreddit(ctx, db.EnsureSubredditParams{
			Name:        sub,
			Title:       sql.NullString{String: sub, Valid: true},
			Description: sql.NullString{String: "", Valid: true},
			Subscribers: sql.NullInt32{Int32: 0, Valid: true},
		})
		if err != nil {
			log.Printf("⚠️ Failed to ensure subreddit %s: %v", sub, err)
			continue
		}
		// A reference may be observed before its target subreddit has ever been
		// crawled. Resolve those durable identities as soon as discovery creates
		// the target so the explicit relationship layer is not permanently lost.
		if _, err := q.DB().ExecContext(ctx, `UPDATE subreddit_cross_references SET target_subreddit_id=$1
WHERE target_subreddit_id IS NULL AND lower(target_subreddit_name)=lower($2)`, subreddit, sub); err != nil {
			log.Printf("⚠️ Failed to resolve stored references for %s: %v", sub, err)
		}

		// Mentions receive a stronger score than author-history evidence, but
		// promotion remains subject to the shared daily budget.
		if err := recordDiscoveryCandidate(ctx, q, subreddit, "mention", "post_text", 10); err != nil {
			log.Printf("⚠️ Failed to record mentioned subreddit %s: %v", sub, err)
		} else {
			enqueuedCount++
		}
	}
	log.Printf("✅ Recorded %d/%d linked subreddit candidates", enqueuedCount, len(linked))
}
