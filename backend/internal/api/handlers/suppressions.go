package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

type SuppressionHandler struct{ db db.DBTX }

func NewSuppressionHandler(database db.DBTX) *SuppressionHandler {
	return &SuppressionHandler{db: database}
}

type suppressionRequest struct {
	EntityID       string `json:"entity_id"`
	EntityType     string `json:"entity_type"`
	ReasonCategory string `json:"reason_category"`
	OperatorNote   string `json:"operator_note"`
}

func (h *SuppressionHandler) List(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := h.db.QueryContext(r.Context(), `SELECT entity_id,entity_type,reason_category,operator_note,created_by,
created_at,restored_by,restored_at FROM public_entity_suppressions
WHERE ($1='' OR entity_id ILIKE '%'||$1||'%') ORDER BY (restored_at IS NULL) DESC,created_at DESC LIMIT 500`, query)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, kind, reason, note, createdBy string
		var createdAt string
		var restoredBy, restoredAt sql.NullString
		if err := rows.Scan(&id, &kind, &reason, &note, &createdBy, &createdAt, &restoredBy, &restoredAt); err != nil {
			writeRevisionError(w, err)
			return
		}
		items = append(items, map[string]any{"entity_id": id, "entity_type": kind, "reason_category": reason, "operator_note": note,
			"created_by": createdBy, "created_at": createdAt, "restored_by": restoredBy, "restored_at": restoredAt, "active": !restoredAt.Valid})
	}
	writeJSON(w, map[string]any{"suppressions": items, "count": len(items)})
}

func (h *SuppressionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var request suppressionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	request.EntityID = strings.TrimSpace(request.EntityID)
	request.EntityType = strings.TrimSpace(request.EntityType)
	request.ReasonCategory = strings.TrimSpace(request.ReasonCategory)
	if request.EntityID == "" || len(request.EntityID) > 256 || !validSuppressionType(request.EntityType) || !validSuppressionReason(request.ReasonCategory) || len(request.OperatorNote) > 4000 {
		http.Error(w, "invalid suppression", http.StatusBadRequest)
		return
	}
	operator := getUserIDFromRequest(r)
	if operator == "" {
		operator = "admin-token"
	}
	tx, ok := h.db.(*sql.DB)
	if !ok {
		http.Error(w, "suppression transaction unavailable", http.StatusServiceUnavailable)
		return
	}
	databaseTx, err := tx.BeginTx(r.Context(), nil)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer databaseTx.Rollback()
	_, err = databaseTx.ExecContext(r.Context(), `INSERT INTO public_entity_suppressions(entity_id,entity_type,reason_category,operator_note,created_by)
VALUES($1,$2,$3,$4,$5) ON CONFLICT(entity_id) DO UPDATE SET entity_type=EXCLUDED.entity_type,reason_category=EXCLUDED.reason_category,
operator_note=EXCLUDED.operator_note,created_by=EXCLUDED.created_by,created_at=now(),restored_by=NULL,restored_at=NULL`,
		request.EntityID, request.EntityType, request.ReasonCategory, request.OperatorNote, operator)
	if err == nil {
		_, err = databaseTx.ExecContext(r.Context(), `INSERT INTO admin_audit_log(action,resource_type,resource_id,user_id,details,ip_address)
VALUES('suppress_public_entity','public_entity_suppression',$1,$2,jsonb_build_object('entity_type',$3::text,'reason_category',$4::text),$5)`,
			request.EntityID, operator, request.EntityType, request.ReasonCategory, getIPFromRequest(r))
	}
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if err := databaseTx.Commit(); err != nil {
		writeRevisionError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"entity_id": request.EntityID, "active": true})
}

func (h *SuppressionHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("entity_id"))
	if id == "" {
		http.Error(w, "entity_id is required", http.StatusBadRequest)
		return
	}
	operator := getUserIDFromRequest(r)
	if operator == "" {
		operator = "admin-token"
	}
	database, ok := h.db.(*sql.DB)
	if !ok {
		http.Error(w, "suppression transaction unavailable", http.StatusServiceUnavailable)
		return
	}
	databaseTx, err := database.BeginTx(r.Context(), nil)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	defer databaseTx.Rollback()
	result, err := databaseTx.ExecContext(r.Context(), `UPDATE public_entity_suppressions SET restored_by=$2,restored_at=now() WHERE entity_id=$1 AND restored_at IS NULL`, id, operator)
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		http.Error(w, "active suppression not found", http.StatusNotFound)
		return
	}
	_, err = databaseTx.ExecContext(r.Context(), `INSERT INTO admin_audit_log(action,resource_type,resource_id,user_id,details,ip_address)
VALUES('restore_public_entity','public_entity_suppression',$1,$2,'{}'::jsonb,$3)`, id, operator, getIPFromRequest(r))
	if err != nil {
		writeRevisionError(w, err)
		return
	}
	if err := databaseTx.Commit(); err != nil {
		writeRevisionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"entity_id": id, "active": false})
}

func validSuppressionType(value string) bool {
	switch value {
	case "community", "subreddit", "user", "post", "comment":
		return true
	}
	return false
}
func validSuppressionReason(value string) bool {
	switch value {
	case "opt_out", "legal", "privacy", "safety", "administrative":
		return true
	}
	return false
}

func isSuppressed(database revisionDB, r *http.Request, entityID string) (bool, error) {
	var suppressed bool
	err := database.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM public_entity_suppressions WHERE entity_id=$1 AND restored_at IS NULL)`, entityID).Scan(&suppressed)
	return suppressed, err
}
