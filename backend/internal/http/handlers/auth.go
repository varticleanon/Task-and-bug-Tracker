package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/http/response"
)

type AuthHandler struct {
	pool         *pgxpool.Pool
	sessionStore *auth.SessionStore
	csrfSecret   []byte
	isProd       bool
}

func NewAuthHandler(pool *pgxpool.Pool, sessionStore *auth.SessionStore, csrfSecret []byte, isProd bool) *AuthHandler {
	return &AuthHandler{
		pool:         pool,
		sessionStore: sessionStore,
		csrfSecret:   csrfSecret,
		isProd:       isProd,
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request payload")
		return
	}

	email := auth.NormalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		response.SendError(w, r, http.StatusBadRequest, "INVALID_INPUT", "Email and password are required")
		return
	}

	query := `
		SELECT id, email, name, password_hash, global_role, is_active, must_change_password
		FROM users
		WHERE lower(email) = $1
	`
	var user struct {
		middleware.AuthUser
		PasswordHash string
	}

	err := h.pool.QueryRow(r.Context(), query, email).Scan(
		&user.ID, &user.Email, &user.Name, &user.PasswordHash,
		&user.GlobalRole, &user.IsActive, &user.MustChangePassword,
	)

	if err != nil {
		response.SendError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}

	if !user.IsActive {
		response.SendError(w, r, http.StatusUnauthorized, "ACCOUNT_DISABLED", "Account is disabled")
		return
	}

	match, err := auth.ComparePassword(req.Password, user.PasswordHash)
	if err != nil || !match {
		response.SendError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}

	tokenStr, session, err := h.sessionStore.CreateSession(r.Context(), user.ID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create session")
		return
	}

	_, _ = h.pool.Exec(r.Context(), "UPDATE users SET last_login_at = NOW() WHERE id = $1", user.ID)

	cookieName := "sid"
	if h.isProd {
		cookieName = "__Host-sid"
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isProd,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.AbsoluteExpiresAt,
	})

	csrfToken := middleware.GenerateCSRFToken(session.ID.String(), h.csrfSecret)
	http.SetCookie(w, &http.Cookie{
		Name:     "XSRF-TOKEN",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   h.isProd,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.AbsoluteExpiresAt,
	})

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"user": user.AuthUser,
		},
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		response.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"user": user,
		},
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session, ok := middleware.GetSessionFromContext(r.Context())
	if ok {
		_ = h.sessionStore.RevokeSession(r.Context(), session.ID)
	}

	cookieName := "sid"
	if h.isProd {
		cookieName = "__Host-sid"
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.isProd,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "XSRF-TOKEN",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   h.isProd,
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		response.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request payload")
		return
	}

	if len(req.NewPassword) < 12 || len(req.NewPassword) > 128 {
		response.SendError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Password must be 12-128 characters")
		return
	}

	var currentHash string
	err := h.pool.QueryRow(r.Context(), "SELECT password_hash FROM users WHERE id = $1", user.ID).Scan(&currentHash)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch user credentials")
		return
	}

	match, err := auth.ComparePassword(req.CurrentPassword, currentHash)
	if err != nil || !match {
		response.SendError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Current password is incorrect")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to hash new password")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), "UPDATE users SET password_hash = $1, must_change_password = false, updated_at = NOW() WHERE id = $2", newHash, user.ID)
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update password")
		return
	}

	_ = h.sessionStore.RevokeAllUserSessions(r.Context(), user.ID)

	if err := tx.Commit(r.Context()); err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to commit transaction")
		return
	}

	tokenStr, session, err := h.sessionStore.CreateSession(r.Context(), user.ID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create new session")
		return
	}

	cookieName := "sid"
	if h.isProd {
		cookieName = "__Host-sid"
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isProd,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.AbsoluteExpiresAt,
	})

	csrfToken := middleware.GenerateCSRFToken(session.ID.String(), h.csrfSecret)
	http.SetCookie(w, &http.Cookie{
		Name:     "XSRF-TOKEN",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   h.isProd,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.AbsoluteExpiresAt,
	})

	user.MustChangePassword = false
	response.SendJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"user":    user,
			"message": "Password successfully updated",
		},
	})
}
