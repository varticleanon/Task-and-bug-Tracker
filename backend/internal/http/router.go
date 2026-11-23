package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/handlers"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/storage"
	"time"
)

func NewRouter(pool *pgxpool.Pool, sessionStore *auth.SessionStore, csrfSecret string, isProd bool, store storage.Storage) http.Handler {
	r := chi.NewRouter()

	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Recoverer)
	r.Use(middleware.OriginValidation)
	r.Use(middleware.Cors)
	r.Use(middleware.StripProxyPrefix)

	// Initialize rate limiter for authentication endpoints
	// SSOT §9: rate limiting for login and password changes (single-instance memory limiter acceptable in MVP)
	rateLimiter := middleware.NewLoginRateLimiter(10, 15*time.Minute)

	healthHandler := handlers.NewHealthHandler(pool)
	r.Get("/health/live", healthHandler.Live)
	r.Get("/health/ready", healthHandler.Ready)

	authHandler := handlers.NewAuthHandler(pool, sessionStore, []byte(csrfSecret), isProd)
	r.Post("/api/v1/auth/login", authHandler.Login)

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(sessionStore, pool, isProd))
		r.Use(middleware.RequireCSRF([]byte(csrfSecret), isProd))
		r.Use(rateLimiter.LimitMiddleware())

		// ... rest of routes

		// ... rest of routes
		r.Get("/api/v1/auth/me", authHandler.Me)
		r.Post("/api/v1/auth/change-password", authHandler.ChangePassword)
		r.Post("/api/v1/auth/logout", authHandler.Logout)

		adminUserHandler := handlers.NewAdminUserHandler(pool, sessionStore)
		r.With(middleware.RequireSuperAdmin).Get("/api/v1/admin/users", adminUserHandler.ListUsers)
		r.With(middleware.RequireSuperAdmin).Post("/api/v1/admin/users", adminUserHandler.CreateUser)
		r.With(middleware.RequireSuperAdmin).Patch("/api/v1/admin/users/{userId}", adminUserHandler.UpdateUser)
		r.With(middleware.RequireSuperAdmin).Post("/api/v1/admin/users/{userId}/reset-password", adminUserHandler.ResetPassword)

		wsHandler := handlers.NewWorkspaceHandler(pool)
		r.Get("/api/v1/workspaces", wsHandler.ListWorkspaces)
		r.With(middleware.RequireSuperAdmin).Post("/api/v1/workspaces", wsHandler.CreateWorkspace)

		r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRole(pool, "VIEWER"))

			r.Get("/", wsHandler.GetWorkspace)
			r.With(middleware.RequireWorkspaceRole(pool, "ADMIN")).Patch("/", wsHandler.UpdateWorkspace)
			r.With(middleware.RequireSuperAdmin).Post("/archive", wsHandler.ArchiveWorkspace)
			r.With(middleware.RequireSuperAdmin).Post("/restore", wsHandler.RestoreWorkspace)

			r.Get("/members", wsHandler.ListMembers)
			r.With(middleware.RequireWorkspaceRole(pool, "ADMIN")).Post("/members", wsHandler.AddMember)
			r.With(middleware.RequireWorkspaceRole(pool, "ADMIN")).Patch("/members/{userId}", wsHandler.UpdateMemberRole)
			r.With(middleware.RequireWorkspaceRole(pool, "ADMIN")).Delete("/members/{userId}", wsHandler.RemoveMember)

			dashHandler := handlers.NewDashboardHandler(pool)
			r.Get("/dashboard", dashHandler.GetDashboardSummary)

			itemHandler := handlers.NewItemHandler(pool)
			r.Get("/items", itemHandler.ListItems)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Post("/items", itemHandler.CreateItem)
			r.Get("/items/{id}", itemHandler.GetItem)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Patch("/items/{id}", itemHandler.UpdateItem)
			r.With(middleware.RequireWorkspaceRole(pool, "ADMIN")).Delete("/items/{id}", itemHandler.SoftDeleteItem)
			r.Get("/items/{id}/activities", itemHandler.GetItemActivities)

			attachHandler := handlers.NewAttachmentHandler(pool, store)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Post("/items/{id}/attachments", attachHandler.UploadAttachment)
			r.Get("/items/{id}/attachments/{aid}/content", attachHandler.GetAttachmentContent)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Delete("/items/{id}/attachments/{aid}", attachHandler.DeleteAttachment)
		})
	})

	return r
}
