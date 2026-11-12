package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/sqlc-dev/pqtype"
	"github.com/subculture-collective/clustr/backend/internal/db"
	"github.com/subculture-collective/clustr/backend/internal/scheduler"
	"github.com/subculture-collective/clustr/backend/internal/utils"
)

type ScheduledJobsHandler struct {
	q *db.Queries
}

func NewScheduledJobsHandler(q *db.Queries) *ScheduledJobsHandler {
	return &ScheduledJobsHandler{q: q}
}

type CreateScheduledJobRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	SubredditID    *int32 `json:"subreddit_id"`
	CronExpression string `json:"cron_expression"`
	Enabled        bool   `json:"enabled"`
	Priority       int32  `json:"priority"`
}

type ScheduledJobResponse struct {
	ID             int32   `json:"id"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	SubredditID    *int32  `json:"subreddit_id"`
	CronExpression string  `json:"cron_expression"`
	Enabled        bool    `json:"enabled"`
	LastRunAt      *string `json:"last_run_at"`
	NextRunAt      string  `json:"next_run_at"`
	Priority       *int32  `json:"priority"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	CreatedBy      *string `json:"created_by"`
}

// CreateScheduledJob creates a new scheduled job
func (h *ScheduledJobsHandler) CreateScheduledJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateScheduledJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate cron expression
	if err := scheduler.ValidateCronExpression(req.CronExpression); err != nil {
		http.Error(w, "Invalid cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Calculate next run time
	nextRun, err := scheduler.ParseCronExpression(req.CronExpression, time.Now())
	if err != nil {
		http.Error(w, "Failed to parse cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	createdBy := getUserIDFromRequest(r)

	var subredditID sql.NullInt32
	if req.SubredditID != nil {
		subredditID = sql.NullInt32{Int32: *req.SubredditID, Valid: true}
	}

	job, err := h.q.CreateScheduledJob(ctx, db.CreateScheduledJobParams{
		Name:           req.Name,
		Description:    sql.NullString{String: req.Description, Valid: req.Description != ""},
		SubredditID:    subredditID,
		CronExpression: req.CronExpression,
		Enabled:        req.Enabled,
		NextRunAt:      nextRun,
		Priority:       sql.NullInt32{Int32: req.Priority, Valid: true},
		CreatedBy:      sql.NullString{String: createdBy, Valid: true},
	})
	if err != nil {
		http.Error(w, "Failed to create scheduled job", http.StatusInternalServerError)
		return
	}

	// Log the action
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_id": job.ID,
		"name":   job.Name,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "create_scheduled_job",
		ResourceType: "scheduled_job",
		ResourceID:   sql.NullString{String: strconv.Itoa(int(job.ID)), Valid: true},
		UserID:       createdBy,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	response := toScheduledJobResponse(job)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ListScheduledJobs lists all scheduled jobs
func (h *ScheduledJobsHandler) ListScheduledJobs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	jobs, err := h.q.ListScheduledJobs(ctx, db.ListScheduledJobsParams{
		Column1: int32(limit),
		Column2: int32(offset),
	})
	if err != nil {
		http.Error(w, "Failed to list scheduled jobs", http.StatusInternalServerError)
		return
	}

	var response []ScheduledJobResponse
	for _, job := range jobs {
		response = append(response, toScheduledJobResponse(job))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetScheduledJob retrieves a specific scheduled job
func (h *ScheduledJobsHandler) GetScheduledJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	job, err := h.q.GetScheduledJob(ctx, int32(jobID))
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Scheduled job not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get scheduled job", http.StatusInternalServerError)
		return
	}

	response := toScheduledJobResponse(job)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateScheduledJob updates an existing scheduled job
func (h *ScheduledJobsHandler) UpdateScheduledJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	var req CreateScheduledJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate cron expression
	if err := scheduler.ValidateCronExpression(req.CronExpression); err != nil {
		http.Error(w, "Invalid cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Calculate next run time
	nextRun, err := scheduler.ParseCronExpression(req.CronExpression, time.Now())
	if err != nil {
		http.Error(w, "Failed to parse cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	err = h.q.UpdateScheduledJob(ctx, db.UpdateScheduledJobParams{
		ID:             int32(jobID),
		Name:           req.Name,
		Description:    sql.NullString{String: req.Description, Valid: req.Description != ""},
		CronExpression: req.CronExpression,
		Enabled:        req.Enabled,
		NextRunAt:      nextRun,
		Priority:       sql.NullInt32{Int32: req.Priority, Valid: true},
	})
	if err != nil {
		http.Error(w, "Failed to update scheduled job", http.StatusInternalServerError)
		return
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_id": jobID,
		"name":   req.Name,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "update_scheduled_job",
		ResourceType: "scheduled_job",
		ResourceID:   sql.NullString{String: jobIDStr, Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "job_id": jobID})
}

// DeleteScheduledJob deletes a scheduled job
func (h *ScheduledJobsHandler) DeleteScheduledJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	err = h.q.DeleteScheduledJob(ctx, int32(jobID))
	if err != nil {
		http.Error(w, "Failed to delete scheduled job", http.StatusInternalServerError)
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
		Action:       "delete_scheduled_job",
		ResourceType: "scheduled_job",
		ResourceID:   sql.NullString{String: jobIDStr, Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// ToggleScheduledJob enables or disables a scheduled job
func (h *ScheduledJobsHandler) ToggleScheduledJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	jobIDStr := vars["id"]

	jobID, err := utils.Atoi32(jobIDStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err = h.q.ToggleScheduledJob(ctx, db.ToggleScheduledJobParams{
		ID:      int32(jobID),
		Enabled: req.Enabled,
	})
	if err != nil {
		http.Error(w, "Failed to toggle scheduled job", http.StatusInternalServerError)
		return
	}

	// Log the action
	userID := getUserIDFromRequest(r)
	ipAddr := getIPFromRequest(r)
	details := map[string]interface{}{
		"job_id":  jobID,
		"enabled": req.Enabled,
	}
	detailsJSON, _ := json.Marshal(details)
	_ = h.q.LogAdminAction(ctx, db.LogAdminActionParams{
		Action:       "toggle_scheduled_job",
		ResourceType: "scheduled_job",
		ResourceID:   sql.NullString{String: jobIDStr, Valid: true},
		UserID:       userID,
		Details:      pqtype.NullRawMessage{RawMessage: detailsJSON, Valid: true},
		IpAddress:    sql.NullString{String: ipAddr, Valid: ipAddr != ""},
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "enabled": req.Enabled})
}

func toScheduledJobResponse(job db.ScheduledJob) ScheduledJobResponse {
	resp := ScheduledJobResponse{
		ID:             job.ID,
		Name:           job.Name,
		CronExpression: job.CronExpression,
		Enabled:        job.Enabled,
		NextRunAt:      job.NextRunAt.Format(time.RFC3339),
		CreatedAt:      job.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:      job.UpdatedAt.Time.Format(time.RFC3339),
	}

	if job.Description.Valid {
		resp.Description = &job.Description.String
	}
	if job.SubredditID.Valid {
		resp.SubredditID = &job.SubredditID.Int32
	}
	if job.LastRunAt.Valid {
		lastRun := job.LastRunAt.Time.Format(time.RFC3339)
		resp.LastRunAt = &lastRun
	}
	if job.Priority.Valid {
		resp.Priority = &job.Priority.Int32
	}
	if job.CreatedBy.Valid {
		resp.CreatedBy = &job.CreatedBy.String
	}

	return resp
}
