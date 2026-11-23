package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/response"
)

type authContextKey string

const (
	UserContextKey    authContextKey = "auth_user"
	SessionContextKey authContextKey = "auth_session"
)

type AuthUser struct {
	ID                 uuid.UUID `json:"id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	GlobalRole         string    `json:"global_role"`
	IsActive           bool      `json:"is_active"`
	MustChangePassword bool      `json:"must_change_password"`
}

func RequireAuth(store *auth.SessionStore, pool *pgxpool.Pool, isProd bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookieName := "sid"
			if isProd {
				cookieName = "__Host-sid"
			}

			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				response.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
				return
			}

			session, err := store.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				response.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired session")
				return
			}

			query := `
				SELECT id, email, name, global_role, is_active, must_change_password
				FROM users
				WHERE id = $1
			`
			var user AuthUser
			err = pool.QueryRow(r.Context(), query, session.UserID).Scan(
				&user.ID, &user.Email, &user.Name,
				&user.GlobalRole, &user.IsActive, &user.MustChangePassword,
			)

			if err != nil || !user.IsActive {
				response.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "User account disabled or not found")
				return
			}

			// SSOT §4: Paksa ganti password sebelum mengakses endpoint selain /auth/me, /auth/change-password, /auth/logout
			if user.MustChangePassword {
				path := r.URL.Path
				allowed := strings.HasSuffix(path, "/auth/me") ||
					strings.HasSuffix(path, "/auth/change-password") ||
					strings.HasSuffix(path, "/auth/logout")
				if !allowed {
					response.SendError(w, r, http.StatusForbidden, "MUST_CHANGE_PASSWORD", "Password change required before accessing this resource")
					return
				}
			}

			ctx := context.WithValue(r.Context(), SessionContextKey, session)
			ctx = context.WithValue(ctx, UserContextKey, &user)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if !ok || user.GlobalRole != "SUPER_ADMIN" {
			response.SendError(w, r, http.StatusForbidden, "FORBIDDEN", "Super Admin privileges required")
			return
		}
		next.ServeHTTP(w, r.WithContext(r.Context()))
	})
}

func GetUserFromContext(ctx context.Context) (*AuthUser, bool) {
	u, ok := ctx.Value(UserContextKey).(*AuthUser)
	return u, ok
}

func GetSessionFromContext(ctx context.Context) (*auth.Session, bool) {
	s, ok := ctx.Value(SessionContextKey).(*auth.Session)
	return s, ok
}
