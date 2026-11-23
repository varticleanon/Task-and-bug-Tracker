package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/http/response"
)

type WorkspaceHandler struct {
	pool *pgxpool.Pool
}

func NewWorkspaceHandler(pool *pgxpool.Pool) *WorkspaceHandler {
	return &WorkspaceHandler{pool: pool}
}

type CreateWorkspaceRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

type UpdateWorkspaceRequest struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
}

type AddMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type UpdateMemberRoleRequest struct {
	Role string `json:"role"`
}

var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type WorkspaceItem struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	CreatedBy   uuid.UUID `json:"created_by"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	ArchivedAt  *string   `json:"archived_at,omitempty"`
}

func (h *WorkspaceHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		response.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var selectQuery string
	var args []interface{}

	if user.GlobalRole == "SUPER_ADMIN" {
		selectQuery = `
			SELECT id, name, slug, description, created_by, created_at, updated_at, archived_at
			FROM workspaces
			ORDER BY created_at DESC
		`
	} else {
		selectQuery = `
			SELECT w.id, w.name, w.slug, w.description, w.created_by, w.created_at, w.updated_at, w.archived_at
			FROM workspaces w
			JOIN workspace_members wm ON w.id = wm.workspace_id
			WHERE wm.user_id = $1 AND w.archived_at IS NULL
			ORDER BY w.created_at DESC
		`
		args = append(args, user.ID)
	}

	rows, err := h.pool.Query(r.Context(), selectQuery, args...)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch workspaces")
		return
	}
	defer rows.Close()

	workspaces := make([]WorkspaceItem, 0)
	for rows.Next() {
		var ws WorkspaceItem
		var createdAt, updatedAt interface{}
		var archivedAt *interface{}
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.CreatedBy, &createdAt, &updatedAt, &archivedAt); err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error scanning workspace row")
			return
		}
		ws.CreatedAt = interfaceToString(createdAt)
		ws.UpdatedAt = interfaceToString(updatedAt)
		if archivedAt != nil && *archivedAt != nil {
			s := interfaceToString(*archivedAt)
			ws.ArchivedAt = &s
		}
		workspaces = append(workspaces, ws)
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": workspaces,
	})
}

func (h *WorkspaceHandler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	query := `
		SELECT id, name, slug, description, created_by, created_at, updated_at, archived_at
		FROM workspaces
		WHERE id = $1
	`
	var ws WorkspaceItem
	var createdAt, updatedAt interface{}
	var archivedAt *interface{}

	err = h.pool.QueryRow(r.Context(), query, wid).Scan(
		&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.CreatedBy, &createdAt, &updatedAt, &archivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Workspace not found")
		return
	} else if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch workspace detail")
		return
	}

	ws.CreatedAt = interfaceToString(createdAt)
	ws.UpdatedAt = interfaceToString(updatedAt)
	if archivedAt != nil && *archivedAt != nil {
		s := interfaceToString(*archivedAt)
		ws.ArchivedAt = &s
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": ws,
	})
}

func (h *WorkspaceHandler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok || user.GlobalRole != "SUPER_ADMIN" {
		response.SendError(w, r, http.StatusForbidden, "FORBIDDEN", "Super Admin privileges required to create workspace")
		return
	}

	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request payload")
		return
	}

	name := strings.TrimSpace(req.Name)
	slug := strings.ToLower(strings.TrimSpace(req.Slug))

	if len(name) < 2 || len(name) > 100 {
		response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Workspace name must be between 2 and 100 characters")
		return
	}
	if len(slug) < 3 || len(slug) > 50 || !slugRegex.MatchString(slug) {
		response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Slug must be 3-50 lowercase alphanumeric characters or hyphens")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	wsID := uuid.New()
	wsQuery := `
		INSERT INTO workspaces (id, name, slug, description, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, name, slug, description, created_by, created_at, updated_at
	`
	var ws struct {
		ID          uuid.UUID   `json:"id"`
		Name        string      `json:"name"`
		Slug        string      `json:"slug"`
		Description string      `json:"description"`
		CreatedBy   uuid.UUID   `json:"created_by"`
		CreatedAt   interface{} `json:"created_at"`
		UpdatedAt   interface{} `json:"updated_at"`
	}

	err = tx.QueryRow(r.Context(), wsQuery, wsID, name, slug, strings.TrimSpace(req.Description), user.ID).Scan(
		&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.CreatedBy, &ws.CreatedAt, &ws.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			response.SendError(w, r, http.StatusConflict, "SLUG_ALREADY_EXISTS", "Workspace slug is already taken")
			return
		}
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create workspace")
		return
	}

	memberQuery := `
		INSERT INTO workspace_members (workspace_id, user_id, role, joined_at, added_by)
		VALUES ($1, $2, 'ADMIN', NOW(), $3)
	`
	_, err = tx.Exec(r.Context(), memberQuery, wsID, user.ID, user.ID)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to add creator as workspace ADMIN")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to commit workspace creation")
		return
	}

	response.SendJSON(w, http.StatusCreated, map[string]interface{}{
		"data": ws,
	})
}

func (h *WorkspaceHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	var req UpdateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request payload")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if len(name) < 2 || len(name) > 100 {
			response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Workspace name must be 2-100 characters")
			return
		}
		_, err = tx.Exec(r.Context(), "UPDATE workspaces SET name = $1, updated_at = NOW() WHERE id = $2", name, wid)
		if err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update workspace name")
			return
		}
	}

	if req.Slug != nil {
		slug := strings.ToLower(strings.TrimSpace(*req.Slug))
		if len(slug) < 3 || len(slug) > 50 || !slugRegex.MatchString(slug) {
			response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid slug format")
			return
		}
		_, err = tx.Exec(r.Context(), "UPDATE workspaces SET slug = $1, updated_at = NOW() WHERE id = $2", slug, wid)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				response.SendError(w, r, http.StatusConflict, "SLUG_ALREADY_EXISTS", "Workspace slug is already taken")
				return
			}
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update workspace slug")
			return
		}
	}

	if req.Description != nil {
		_, err = tx.Exec(r.Context(), "UPDATE workspaces SET description = $1, updated_at = NOW() WHERE id = $2", strings.TrimSpace(*req.Description), wid)
		if err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update description")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to commit update")
		return
	}

	h.GetWorkspace(w, r)
}

func (h *WorkspaceHandler) ArchiveWorkspace(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok || user.GlobalRole != "SUPER_ADMIN" {
		response.SendError(w, r, http.StatusForbidden, "FORBIDDEN", "Only Super Admin can archive workspaces")
		return
	}

	res, err := h.pool.Exec(r.Context(), "UPDATE workspaces SET archived_at = NOW(), updated_at = NOW() WHERE id = $1 AND archived_at IS NULL", wid)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to archive workspace")
		return
	}

	if res.RowsAffected() == 0 {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Workspace not found or already archived")
		return
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Workspace successfully archived",
	})
}

func (h *WorkspaceHandler) RestoreWorkspace(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok || user.GlobalRole != "SUPER_ADMIN" {
		response.SendError(w, r, http.StatusForbidden, "FORBIDDEN", "Only Super Admin can restore workspaces")
		return
	}

	res, err := h.pool.Exec(r.Context(), "UPDATE workspaces SET archived_at = NULL, updated_at = NOW() WHERE id = $1 AND archived_at IS NOT NULL", wid)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to restore workspace")
		return
	}

	if res.RowsAffected() == 0 {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Archived workspace not found")
		return
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Workspace successfully restored",
	})
}

func (h *WorkspaceHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	query := `
		SELECT u.id, u.email, u.name, wm.role, wm.joined_at
		FROM workspace_members wm
		JOIN users u ON u.id = wm.user_id
		WHERE wm.workspace_id = $1 AND u.is_active = true
		ORDER BY wm.joined_at ASC
	`
	rows, err := h.pool.Query(r.Context(), query, wid)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch workspace members")
		return
	}
	defer rows.Close()

	type MemberItem struct {
		UserID   uuid.UUID `json:"user_id"`
		Email    string    `json:"email"`
		Name     string    `json:"name"`
		Role     string    `json:"role"`
		JoinedAt string    `json:"joined_at"`
	}

	members := make([]MemberItem, 0)
	for rows.Next() {
		var m MemberItem
		var joinedAt interface{}
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &joinedAt); err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error scanning member row")
			return
		}
		m.JoinedAt = interfaceToString(joinedAt)
		members = append(members, m)
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": members,
	})
}

func (h *WorkspaceHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	actor, _ := middleware.GetUserFromContext(r.Context())
	var req AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request payload")
		return
	}

	email := auth.NormalizeEmail(req.Email)
	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if role != "ADMIN" && role != "MEMBER" && role != "VIEWER" {
		response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Role must be ADMIN, MEMBER, or VIEWER")
		return
	}

	var targetUser struct {
		ID       uuid.UUID
		IsActive bool
	}
	err = h.pool.QueryRow(r.Context(), "SELECT id, is_active FROM users WHERE lower(email) = $1", email).Scan(&targetUser.ID, &targetUser.IsActive)
	if errors.Is(err, pgx.ErrNoRows) || !targetUser.IsActive {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Active user with exact email not found")
		return
	}

	var actorID *uuid.UUID
	if actor != nil {
		actorID = &actor.ID
	}

	_, err = h.pool.Exec(r.Context(),
		"INSERT INTO workspace_members (workspace_id, user_id, role, joined_at, added_by) VALUES ($1, $2, $3, NOW(), $4)",
		wid, targetUser.ID, role, actorID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "primary key") {
			response.SendError(w, r, http.StatusConflict, "MEMBER_ALREADY_EXISTS", "User is already a member of this workspace")
			return
		}
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to add workspace member")
		return
	}

	response.SendJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Member successfully added",
	})
}

func (h *WorkspaceHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	targetUserIDStr := chi.URLParam(r, "userId")
	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid target user ID")
		return
	}

	var req UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request payload")
		return
	}

	newRole := strings.ToUpper(strings.TrimSpace(req.Role))
	if newRole != "ADMIN" && newRole != "MEMBER" && newRole != "VIEWER" {
		response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Role must be ADMIN, MEMBER, or VIEWER")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	var currentRole string
	err = tx.QueryRow(r.Context(), "SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2 FOR UPDATE", wid, targetUserID).Scan(&currentRole)
	if errors.Is(err, pgx.ErrNoRows) {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Workspace member not found")
		return
	}

	if currentRole == "ADMIN" && newRole != "ADMIN" {
		var adminCount int
		err = tx.QueryRow(r.Context(), "SELECT COUNT(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'ADMIN'", wid).Scan(&adminCount)
		if err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count workspace ADMINs")
			return
		}
		if adminCount <= 1 {
			response.SendError(w, r, http.StatusConflict, "LAST_WORKSPACE_ADMIN_GUARD", "Cannot demote the last ADMIN of an active workspace")
			return
		}
	}

	_, err = tx.Exec(r.Context(), "UPDATE workspace_members SET role = $1 WHERE workspace_id = $2 AND user_id = $3", newRole, wid, targetUserID)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update member role")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to commit role change")
		return
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Member role updated",
	})
}

func (h *WorkspaceHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	widStr := chi.URLParam(r, "workspaceId")
	wid, err := uuid.Parse(widStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID")
		return
	}

	targetUserIDStr := chi.URLParam(r, "userId")
	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid target user ID")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	var targetRole string
	err = tx.QueryRow(r.Context(), "SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2 FOR UPDATE", wid, targetUserID).Scan(&targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Workspace member not found")
		return
	}

	if targetRole == "ADMIN" {
		var adminCount int
		err = tx.QueryRow(r.Context(), "SELECT COUNT(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'ADMIN'", wid).Scan(&adminCount)
		if err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count workspace ADMINs")
			return
		}
		if adminCount <= 1 {
			response.SendError(w, r, http.StatusConflict, "LAST_WORKSPACE_ADMIN_GUARD", "Cannot remove the last ADMIN of an active workspace")
			return
		}
	}

	_, err = tx.Exec(r.Context(), "DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2", wid, targetUserID)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to remove member")
		return
	}

	_, err = tx.Exec(r.Context(), "UPDATE work_items SET assignee_id = NULL WHERE workspace_id = $1 AND assignee_id = $2", wid, targetUserID)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to unassign work items")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to commit member removal")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func interfaceToString(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return ""
	}
}
