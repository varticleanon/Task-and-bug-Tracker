package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/http/response"
)

type AdminUserHandler struct {
	pool         *pgxpool.Pool
	sessionStore *auth.SessionStore
}

func NewAdminUserHandler(pool *pgxpool.Pool, sessionStore *auth.SessionStore) *AdminUserHandler {
	return &AdminUserHandler{
		pool:         pool,
		sessionStore: sessionStore,
	}
}

func generateTempPassword() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "Tmp#" + hex.EncodeToString(b), nil
}

type CreateUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type UpdateUserRequest struct {
	Name     *string `json:"name,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

func (h *AdminUserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))

	offset := (page - 1) * pageSize

	var countQuery string
	var args []interface{}

	if search != "" {
		countQuery = `SELECT COUNT(*) FROM users WHERE lower(email) LIKE $1 OR lower(name) LIKE $1`
		args = append(args, "%"+strings.ToLower(search)+"%")
	} else {
		countQuery = `SELECT COUNT(*) FROM users`
	}

	var total int
	err := h.pool.QueryRow(r.Context(), countQuery, args...).Scan(&total)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count users")
		return
	}

	var selectQuery string
	if search != "" {
		selectQuery = `
			SELECT id, email, name, global_role, is_active, must_change_password, created_at, updated_at
			FROM users
			WHERE lower(email) LIKE $1 OR lower(name) LIKE $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = []interface{}{"%" + strings.ToLower(search) + "%", pageSize, offset}
	} else {
		selectQuery = `
			SELECT id, email, name, global_role, is_active, must_change_password, created_at, updated_at
			FROM users
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
		args = []interface{}{pageSize, offset}
	}

	rows, err := h.pool.Query(r.Context(), selectQuery, args...)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch users")
		return
	}
	defer rows.Close()

	type UserListItem struct {
		ID                 uuid.UUID `json:"id"`
		Email              string    `json:"email"`
		Name               string    `json:"name"`
		GlobalRole         string    `json:"global_role"`
		IsActive           bool      `json:"is_active"`
		MustChangePassword bool      `json:"must_change_password"`
		CreatedAt          string    `json:"created_at"`
		UpdatedAt          string    `json:"updated_at"`
	}

	users := make([]UserListItem, 0)
	for rows.Next() {
		var u UserListItem
		var createdAt, updatedAt interface{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.GlobalRole, &u.IsActive, &u.MustChangePassword, &createdAt, &updatedAt); err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error scanning user row")
			return
		}
		u.CreatedAt = interfaceToString(createdAt)
		u.UpdatedAt = interfaceToString(updatedAt)
		users = append(users, u)
	}

	totalPages := (total + pageSize - 1) / pageSize

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": users,
		"meta": map[string]interface{}{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

func (h *AdminUserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request payload")
		return
	}

	normalizedEmail := auth.NormalizeEmail(req.Email)
	if normalizedEmail == "" || strings.TrimSpace(req.Name) == "" {
		response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Email and name are required")
		return
	}

	tempPassword, err := generateTempPassword()
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate temporary password")
		return
	}

	hashedPassword, err := auth.HashPassword(tempPassword)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to hash password")
		return
	}

	newID := uuid.New()
	query := `
		INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'USER', true, true, NOW(), NOW())
		RETURNING id, email, name, global_role, is_active, must_change_password
	`

	var user middleware.AuthUser
	err = h.pool.QueryRow(r.Context(), query, newID, normalizedEmail, strings.TrimSpace(req.Name), hashedPassword).Scan(
		&user.ID, &user.Email, &user.Name, &user.GlobalRole, &user.IsActive, &user.MustChangePassword,
	)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			response.SendError(w, r, http.StatusConflict, "USER_ALREADY_EXISTS", "User with this email already exists")
			return
		}
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create user")
		return
	}

	actor, _ := middleware.GetUserFromContext(r.Context())
	var actorID *uuid.UUID
	if actor != nil {
		actorID = &actor.ID
	}
	auditQuery := `
		INSERT INTO audit_events (actor_user_id, entity_type, entity_id, action, details, request_id, created_at)
		VALUES ($1, 'user', $2, 'USER_CREATED', $3, $4, NOW())
	`
	details, _ := json.Marshal(map[string]string{"email": user.Email, "name": user.Name})
	_, _ = h.pool.Exec(r.Context(), auditQuery, actorID, user.ID, details, response.GetRequestID(r.Context()))

	response.SendJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"user":               user,
			"temporary_password": tempPassword,
		},
	})
}

func (h *AdminUserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	targetIDStr := chi.URLParam(r, "userId")
	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid target user ID")
		return
	}

	var req UpdateUserRequest
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

	var targetRole string
	var currentActive bool
	err = tx.QueryRow(r.Context(), "SELECT global_role, is_active FROM users WHERE id = $1 FOR UPDATE", targetID).Scan(&targetRole, &currentActive)
	if errors.Is(err, pgx.ErrNoRows) {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	} else if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to lock target user")
		return
	}

	// SSOT §3: Guard last active Super Admin dari pembatalan (disabling)
	if req.IsActive != nil && !*req.IsActive && currentActive {
		if targetRole == "SUPER_ADMIN" {
			var activeSuperAdmins int
			countErr := tx.QueryRow(r.Context(), "SELECT COUNT(*) FROM users WHERE global_role = 'SUPER_ADMIN' AND is_active = true").Scan(&activeSuperAdmins)
			if countErr != nil {
				response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count active Super Admins")
				return
			}
			if activeSuperAdmins <= 1 {
				response.SendError(w, r, http.StatusConflict, "LAST_SUPER_ADMIN_GUARD", "Cannot disable the last active Super Admin")
				return
			}
		}
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		_, err = tx.Exec(r.Context(), "UPDATE users SET name = $1, updated_at = NOW() WHERE id = $2", strings.TrimSpace(*req.Name), targetID)
		if err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update user name")
			return
		}
	}

	if req.IsActive != nil {
		_, err = tx.Exec(r.Context(), "UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2", *req.IsActive, targetID)
		if err != nil {
			response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update user active status")
			return
		}

		if !*req.IsActive {
			// Cabut seluruh session aktif user yang dinonaktifkan
			_ = h.sessionStore.RevokeAllUserSessions(r.Context(), targetID)

			// SSOT §3 Rule 5: Unassign seluruh work items yang ditugaskan ke user ini
			_, _ = tx.Exec(r.Context(), "UPDATE work_items SET assignee_id = NULL WHERE assignee_id = $1", targetID)
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to commit user update")
		return
	}

	var updatedUser middleware.AuthUser
	_ = h.pool.QueryRow(r.Context(), "SELECT id, email, name, global_role, is_active, must_change_password FROM users WHERE id = $1", targetID).Scan(
		&updatedUser.ID, &updatedUser.Email, &updatedUser.Name, &updatedUser.GlobalRole, &updatedUser.IsActive, &updatedUser.MustChangePassword,
	)

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": updatedUser,
	})
}

func (h *AdminUserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	targetIDStr := chi.URLParam(r, "userId")
	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid target user ID")
		return
	}

	tempPassword, err := generateTempPassword()
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate temporary password")
		return
	}

	hashedPassword, err := auth.HashPassword(tempPassword)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to hash password")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	res, err := tx.Exec(r.Context(),
		"UPDATE users SET password_hash = $1, must_change_password = true, updated_at = NOW() WHERE id = $2 AND is_active = true",
		hashedPassword, targetID,
	)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reset password")
		return
	}

	if res.RowsAffected() == 0 {
		response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Active user not found")
		return
	}

	// SSOT §4: Cabut seluruh session user yang direset passwordnya oleh Admin
	_ = h.sessionStore.RevokeAllUserSessions(r.Context(), targetID)

	if err := tx.Commit(r.Context()); err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to commit password reset")
		return
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"message":            "Password successfully reset",
			"temporary_password": tempPassword,
		},
	})
}
