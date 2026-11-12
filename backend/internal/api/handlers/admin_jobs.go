package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
	"github.com/onnwee/reddit-cluster-map/backend/internal/utils"
	"github.com/sqlc-dev/pqtype"
)

type AdminJobsHandler struct {
	q *db.Queries
}

func NewAdminJobsHandler(q *db.Queries) *AdminJobsHandler {
	return &AdminJobsHandler{q: q}
}

// JobStatsResponse represents job queue statistics
type JobStatsResponse struct {
	QueuedCount    int64 `json:"queued_count"`
	RunningCount   int64 `json:"running_count"`
	FailedCount    int64 `json:"failed_count"`
	CompletedCount int64 `json:"completed_count"`
	TotalCount     int64 `json:"total_count"`
}

// JobResponse represents a single crawl job with subreddit name
type JobResponse struct {
	ID            int32   `json:"id"`
	SubredditID   int32   `json:"subreddit_id"`
	SubredditName string  `json:"subreddit_name"`
	Status        string  `json:"status"`
	Retries       *int32  `json:"retries"`
	Priority      *int32  `json:"priority"`
	LastAttempt   *string `json:"last_attempt"`
	EnqueuedBy    *string `json:"enqueued_by"`
	CreatedAt     *string `json:"created_at"`
	UpdatedAt     *string `json:"updated_at"`
}

// GetJobStats returns statistics about crawl jobs
func (h *AdminJobsHandler) GetJobStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.q.GetAdminCrawlJobStats(ctx)
	if err != nil {
		http.Error(w, "Failed to fetch job stats", http.StatusInternalServerError)
		return
	}

	response := JobStatsResponse{
		QueuedCount:    stats.QueuedCount,
		RunningCount:   stats.RunningCount,
		FailedCount:    stats.FailedCount,
		CompletedCount: stats.CompletedCount,
		TotalCount:     stats.TotalCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListJobsByStatus lists jobs filtered by status
func (h *AdminJobsHandler) ListJobsByStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")

	if status == "" {
		http.Error(w, "status parameter is required", http.StatusBadRequest)
		return
	}

	limit := 100
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

	jobs, err := h.q.ListCrawlJobsByStatus(ctx, db.ListCrawlJobsByStatusParams{
		Status:  status,
		Column2: int32(limit),
		Column3: int32(offset),
	})
	if err != nil {
		http.Error(w, "Failed to fetch jobs", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	var response []JobResponse
	for _, job := range jobs {
		jr := JobResponse{
			ID:            job.ID,
			SubredditID:   job.SubredditID,
			SubredditName: job.SubredditName,
			Status:        job.Status,
		}
		if job.Retries.Valid {
			retries := job.Retries.Int32
			jr.Retries = &retries
		}
		if job.Priority.Valid {
			priority := job.Priority.Int32
			jr.Priority = &priority
		}
		if job.LastAttempt.Valid {
			lastAttempt := job.LastAttempt.Time.Format("2006-01-02T15:04:05Z")
			jr.LastAttempt = &lastAttempt
		}
		if job.EnqueuedBy.Valid {
			jr.EnqueuedBy = &job.EnqueuedBy.String
		}
		if job.CreatedAt.Valid {
			createdAt := job.CreatedAt.Time.Format("2006-01-02T15:04:05Z")
			jr.CreatedAt = &createdAt
		}
		if job.UpdatedAt.Valid {
			updatedAt := job.UpdatedAt.Time.Format("2006-01-02T15:04:05Z")
			jr.UpdatedAt = &updatedAt
		}
		response = append(response, jr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateJobStatus updates the status of a crawl job
func (h *AdminJobsHandler) UpdateJobStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"queued":   true,
		"crawling": true,
		"success":  true,
		"failed":   true,
	}
	if !validStatuses[req.Status] {
		http.Error(w, "Invalid status value", http.StatusBadRequest)
		return
	}

	// Update the job status
	err = h.q.UpdateCrawlJobStatus(ctx, db.UpdateCrawlJobStatusParams{
		ID:     int32(jobID),
		Status: req.Status,
	})
	if err != nil {
		http.Error(w, "Failed to update job status", http.StatusInternalServerError)
		return
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_id":     jobID,
		"new_status": req.Status,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "update_job_status",
		ResourceType: "crawl_job",
		ResourceID:   sql.NullString{String: jobIDStr, Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "job_id": jobID})
}

// UpdateJobPriority updates the priority of a crawl job
func (h *AdminJobsHandler) UpdateJobPriority(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Priority int32 `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update the job priority
	err = h.q.UpdateCrawlJobPriority(ctx, db.UpdateCrawlJobPriorityParams{
		ID:       int32(jobID),
		Priority: sql.NullInt32{Int32: req.Priority, Valid: true},
	})
	if err != nil {
		http.Error(w, "Failed to update job priority", http.StatusInternalServerError)
		return
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_id":       jobID,
		"new_priority": req.Priority,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "update_job_priority",
		ResourceType: "crawl_job",
		ResourceID:   sql.NullString{String: jobIDStr, Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "job_id": jobID})
}

// RetryJob retries a failed crawl job
func (h *AdminJobsHandler) RetryJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	// Retry the job (reset status to queued and retries to 0)
	err = h.q.RetryCrawlJob(ctx, int32(jobID))
	if err != nil {
		http.Error(w, "Failed to retry job", http.StatusInternalServerError)
		return
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_id": jobID,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "retry_job",
		ResourceType: "crawl_job",
		ResourceID:   sql.NullString{String: jobIDStr, Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "job_id": jobID})
}

// BulkUpdateJobStatus updates status for multiple jobs at once
func (h *AdminJobsHandler) BulkUpdateJobStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		JobIDs []int32 `json:"job_ids"`
		Status string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"queued":   true,
		"crawling": true,
		"success":  true,
		"failed":   true,
	}
	if !validStatuses[req.Status] {
		http.Error(w, "Invalid status value", http.StatusBadRequest)
		return
	}

	successCount := 0
	for _, jobID := range req.JobIDs {
		err := h.q.UpdateCrawlJobStatus(ctx, db.UpdateCrawlJobStatusParams{
			ID:     jobID,
			Status: req.Status,
		})
		if err == nil {
			successCount++
		}
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_ids":       req.JobIDs,
		"new_status":    req.Status,
		"success_count": successCount,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "bulk_update_job_status",
		ResourceType: "crawl_job",
		ResourceID:   sql.NullString{String: "bulk", Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":            true,
		"success_count": successCount,
		"total_count":   len(req.JobIDs),
	})
}

// BulkRetryJobs retries multiple failed jobs at once
func (h *AdminJobsHandler) BulkRetryJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		JobIDs []int32 `json:"job_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	successCount := 0
	for _, jobID := range req.JobIDs {
		err := h.q.RetryCrawlJob(ctx, jobID)
		if err == nil {
			successCount++
		}
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_ids":       req.JobIDs,
		"success_count": successCount,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "bulk_retry_jobs",
		ResourceType: "crawl_job",
		ResourceID:   sql.NullString{String: "bulk", Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":            true,
		"success_count": successCount,
		"total_count":   len(req.JobIDs),
	})
}

// BoostJobPriority boosts priority for a specific job
func (h *AdminJobsHandler) BoostJobPriority(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Boost int32 `json:"boost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get current job to calculate new priority
	job, err := h.q.GetCrawlJobByID(ctx, int32(jobID))
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get job", http.StatusInternalServerError)
		return
	}

	currentPriority := int32(0)
	if job.Priority.Valid {
		currentPriority = job.Priority.Int32
	}

	newPriority := currentPriority + req.Boost
	if newPriority > 100 {
		newPriority = 100
	}

	// Update the job priority
	err = h.q.UpdateCrawlJobPriority(ctx, db.UpdateCrawlJobPriorityParams{
		ID:       int32(jobID),
		Priority: sql.NullInt32{Int32: newPriority, Valid: true},
	})
	if err != nil {
		http.Error(w, "Failed to boost job priority", http.StatusInternalServerError)
		return
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_id":            jobID,
		"boost":             req.Boost,
		"previous_priority": currentPriority,
		"new_priority":      newPriority,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "boost_job_priority",
		ResourceType: "crawl_job",
		ResourceID:   sql.NullString{String: jobIDStr, Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":           true,
		"job_id":       jobID,
		"new_priority": newPriority,
	})
}

// Helper functions
func getUserIDFromRequest(r *http.Request) string {
	// Extract user identifier from the authorization header
	// The admin token itself can serve as a user identifier for audit purposes
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
		// Use the last 8 characters of the token as a user identifier
		token := auth[len(prefix):]
		if len(token) > 8 {
			return "admin_" + token[len(token)-8:]
		}
		return "admin_" + token
	}
	return "admin_unknown"
}

func getIPFromRequest(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}
