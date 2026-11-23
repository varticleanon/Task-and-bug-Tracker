package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/http/response"
)

type contextKey string

const WorkspaceRoleKey contextKey = "workspaceRole"

func RequireWorkspaceRole(pool *pgxpool.Pool, minRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				response.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
				return
			}

			widStr := chi.URLParam(r, "workspaceId")
			if widStr == "" {
				widStr = chi.URLParam(r, "wid")
			}
			wid, err := uuid.Parse(widStr)
			if err != nil {
				response.SendError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid workspace ID in URL")
				return
			}

			// Super Admin Bypass (SSOT §3)
			if user.GlobalRole == "SUPER_ADMIN" {
				ctx := context.WithValue(r.Context(), WorkspaceRoleKey, "SUPER_ADMIN")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			var wsRole string
			var archivedAt *string
			query := `
				SELECT wm.role, w.archived_at
				FROM workspace_members wm
				JOIN workspaces w ON w.id = wm.workspace_id
				WHERE wm.workspace_id = $1 AND wm.user_id = $2
			`
			err = pool.QueryRow(r.Context(), query, wid, user.ID).Scan(&wsRole, &archivedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				response.SendError(w, r, http.StatusNotFound, "NOT_FOUND", "Workspace not found or access denied")
				return
			} else if err != nil {
				response.SendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to verify workspace membership")
				return
			}

			if archivedAt != nil && r.Method != http.MethodGet {
				response.SendError(w, r, http.StatusForbidden, "WORKSPACE_ARCHIVED", "Archived workspace is read-only")
				return
			}

			if !isRoleAllowed(wsRole, minRole) {
				response.SendError(w, r, http.StatusForbidden, "FORBIDDEN", "Insufficient workspace permissions")
				return
			}

			ctx := context.WithValue(r.Context(), WorkspaceRoleKey, wsRole)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetWorkspaceRoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(WorkspaceRoleKey).(string)
	return role, ok
}

func isRoleAllowed(actualRole, minRole string) bool {
	levels := map[string]int{
		"VIEWER": 1,
		"MEMBER": 2,
		"ADMIN":  3,
	}
	return levels[actualRole] >= levels[minRole]
}
