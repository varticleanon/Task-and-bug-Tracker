package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/http/response"
)

type DashboardResponse struct {
	TotalOpen    int `json:"total_open"`
	Todo         int `json:"todo"`
	InProgress   int `json:"in_progress"`
	Blocked      int `json:"blocked"`
	InReview     int `json:"in_review"`
	Done         int `json:"done"`
	Bugs         int `json:"bugs"`
	Tasks        int `json:"tasks"`
	AssignedToMe int `json:"assigned_to_me"`
	Overdue      int `json:"overdue"`
}

type DashboardHandler struct {
	pool *pgxpool.Pool
}

func NewDashboardHandler(pool *pgxpool.Pool) *DashboardHandler {
	return &DashboardHandler{pool: pool}
}

func (h *DashboardHandler) GetDashboardSummary(w http.ResponseWriter, r *http.Request) {
	wsIDStr := chi.URLParam(r, "workspaceId")
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Workspace ID tidak valid")
		return
	}

	actor, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak ditemukan")
		return
	}

	ctx := r.Context()
	var resp DashboardResponse
	today := time.Now().UTC().Format("2006-01-02")

	query := `
		SELECT 
			COUNT(*) FILTER (WHERE status != 'DONE') AS total_open,
			COUNT(*) FILTER (WHERE status = 'TODO') AS todo,
			COUNT(*) FILTER (WHERE status = 'IN_PROGRESS') AS in_progress,
			COUNT(*) FILTER (WHERE status = 'BLOCKED') AS blocked,
			COUNT(*) FILTER (WHERE status = 'IN_REVIEW') AS in_review,
			COUNT(*) FILTER (WHERE status = 'DONE') AS done,
			COUNT(*) FILTER (WHERE type = 'BUG') AS bugs,
			COUNT(*) FILTER (WHERE type = 'TASK') AS tasks,
			COUNT(*) FILTER (WHERE assignee_id = $2) AS assigned_to_me,
			COUNT(*) FILTER (WHERE due_date < $3::date AND status != 'DONE') AS overdue
		FROM work_items
		WHERE workspace_id = $1 AND deleted_at IS NULL
	`

	err = h.pool.QueryRow(ctx, query, wsID, actor.ID, today).Scan(
		&resp.TotalOpen,
		&resp.Todo,
		&resp.InProgress,
		&resp.Blocked,
		&resp.InReview,
		&resp.Done,
		&resp.Bugs,
		&resp.Tasks,
		&resp.AssignedToMe,
		&resp.Overdue,
	)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		response.Error(w, r, http.StatusInternalServerError, "DB_ERROR", "Gagal mengambil ringkasan dashboard")
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{"data": resp})
}
