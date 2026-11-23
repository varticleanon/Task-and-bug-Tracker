package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"task-bug-tracker/backend/internal/http/response"
)

type ActivityResponse struct {
	ID          int64           `json:"id"`
	ActorUserID *uuid.UUID      `json:"actor_user_id"`
	ActorName   string          `json:"actor_name"`
	Action      string          `json:"action"`
	Details     json.RawMessage `json:"details"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (h *ItemHandler) GetItemActivities(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	itemStr := chi.URLParam(r, "id")

	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Workspace ID tidak valid")
		return
	}
	itemID, err := uuid.Parse(itemStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_ITEM_ID", "Item ID tidak valid")
		return
	}

	ctx := r.Context()

	var exists bool
	err = h.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM work_items WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL)", itemID, wsID).Scan(&exists)
	if err != nil || !exists {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan")
		return
	}

	query := `
		SELECT ae.id, ae.actor_user_id, COALESCE(u.name, 'System') as actor_name, ae.action, ae.details, ae.created_at
		FROM audit_events ae
		LEFT JOIN users u ON u.id = ae.actor_user_id
		WHERE ae.workspace_id = $1 AND ae.entity_id = $2 AND ae.entity_type IN ('ITEM', 'WORK_ITEM')
		ORDER BY ae.created_at DESC, ae.id DESC
	`

	rows, err := h.pool.Query(ctx, query, wsID, itemID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal mengambil riwayat aktivitas")
		return
	}
	defer rows.Close()

	activities := make([]ActivityResponse, 0)
	for rows.Next() {
		var act ActivityResponse
		var detailsBytes []byte
		if err := rows.Scan(&act.ID, &act.ActorUserID, &act.ActorName, &act.Action, &detailsBytes, &act.CreatedAt); err != nil {
			response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal membaca riwayat aktivitas")
			return
		}
		act.Details = json.RawMessage(detailsBytes)
		activities = append(activities, act)
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{"data": activities})
}
