package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/storage"
)

func setupFullTestRouter(pool *pgxpool.Pool, sessionStore *auth.SessionStore, store storage.Storage) http.Handler {
	r := chi.NewRouter()

	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Recoverer)
	r.Use(middleware.OriginValidation)

	healthHandler := NewHealthHandler(pool)
	r.Get("/health/live", healthHandler.Live)
	r.Get("/health/ready", healthHandler.Ready)

	authHandler := NewAuthHandler(pool, sessionStore, []byte("test-csrf-secret"), true)
	r.Post("/api/v1/auth/login", authHandler.Login)

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(sessionStore, pool, false))
		r.Get("/api/v1/auth/me", authHandler.Me)
		r.Post("/api/v1/auth/change-password", authHandler.ChangePassword)
		r.Post("/api/v1/auth/logout", authHandler.Logout)

		adminUserHandler := NewAdminUserHandler(pool, sessionStore)
		r.With(middleware.RequireSuperAdmin).Get("/api/v1/admin/users", adminUserHandler.ListUsers)
		r.With(middleware.RequireSuperAdmin).Post("/api/v1/admin/users", adminUserHandler.CreateUser)
		r.With(middleware.RequireSuperAdmin).Patch("/api/v1/admin/users/{userId}", adminUserHandler.UpdateUser)
		r.With(middleware.RequireSuperAdmin).Post("/api/v1/admin/users/{userId}/reset-password", adminUserHandler.ResetPassword)

		wsHandler := NewWorkspaceHandler(pool)
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

			dashHandler := NewDashboardHandler(pool)
			r.Get("/dashboard", dashHandler.GetDashboardSummary)

			itemHandler := NewItemHandler(pool)
			r.Get("/items", itemHandler.ListItems)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Post("/items", itemHandler.CreateItem)
			r.Get("/items/{id}", itemHandler.GetItem)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Patch("/items/{id}", itemHandler.UpdateItem)
			r.With(middleware.RequireWorkspaceRole(pool, "ADMIN")).Delete("/items/{id}", itemHandler.SoftDeleteItem)
			r.Get("/items/{id}/activities", itemHandler.GetItemActivities)

			attachHandler := NewAttachmentHandler(pool, store)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Post("/items/{id}/attachments", attachHandler.UploadAttachment)
			r.Get("/items/{id}/attachments/{aid}/content", attachHandler.GetAttachmentContent)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Delete("/items/{id}/attachments/{aid}", attachHandler.DeleteAttachment)
		})
	})

	return r
}

func TestPhase4_WorkspaceIsolationRegressionSuite(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping Phase 4 Workspace Isolation Regression test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	router := setupFullTestRouter(pool, sessionStore, nil)

	// WS-01 & SEC-01: Data isolation when switching workspaces & cross-tenant access rejection
	t.Run("WS-01_SEC-01_WorkspaceSwitchingAndCrossTenantAccess", func(t *testing.T) {
		uID := uuid.New()
		pHash, _ := auth.HashPassword("Password123!")
		uEmail := "ws01_" + uID.String()[:8] + "@example.com"
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'WS01 User', $3, 'USER', true, false)", uID, uEmail, pHash)

		wsA := uuid.New()
		wsB := uuid.New()
		_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Workspace A', $2, 'WS A', $3)", wsA, "wsa-"+uID.String()[:8], uID)
		_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Workspace B', $2, 'WS B', $3)", wsB, "wsb-"+uID.String()[:8], uID)

		_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsA, uID)
		_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsB, uID)

		itemA := uuid.New()
		_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Item in WS A', 'TODO', 'MEDIUM', $3, $3)", itemA, wsA, uID)

		token, _, _ := sessionStore.CreateSession(ctx, uID, "127.0.0.1", "TestAgent")

		// Fetch items from WS A -> Must find item A
		reqA := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items", wsA), nil)
		reqA.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqA.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wA := httptest.NewRecorder()
		router.ServeHTTP(wA, reqA)
		if wA.Code != http.StatusOK {
			t.Fatalf("expected 200 GET WS A items, got %d. Body: %s", wA.Code, wA.Body.String())
		}
		if !bytes.Contains(wA.Body.Bytes(), []byte(itemA.String())) {
			t.Errorf("expected item A (%s) in WS A item list response", itemA.String())
		}

		// Fetch items from WS B -> Must NOT include item A
		reqB := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items", wsB), nil)
		reqB.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqB.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wB := httptest.NewRecorder()
		router.ServeHTTP(wB, reqB)
		if wB.Code != http.StatusOK {
			t.Fatalf("expected 200 GET WS B items, got %d", wB.Code)
		}
		if bytes.Contains(wB.Body.Bytes(), []byte(itemA.String())) {
			t.Errorf("data leak detected: item from WS A returned in WS B list")
		}

		// Direct fetch item A using WS B path -> Must return 404
		reqCross := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsB, itemA), nil)
		reqCross.AddCookie(&http.Cookie{Name: "sid", Value: token})
		reqCross.AddCookie(&http.Cookie{Name: "__Host-sid", Value: token})
		wCross := httptest.NewRecorder()
		router.ServeHTTP(wCross, reqCross)
		if wCross.Code != http.StatusNotFound {
			t.Errorf("expected 404 Not Found on cross-tenant item request, got %d", wCross.Code)
		}
	})

	// WS-02: Member removal revokes workspace access and nullifies assigned PIC in atomic transaction
	t.Run("WS-02_MemberRemovalRevokesAccessAndNullifiesPIC", func(t *testing.T) {
		adminID := uuid.New()
		memberID := uuid.New()
		pHash, _ := auth.HashPassword("Password123!")
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Admin User', $3, 'USER', true, false)", adminID, "ws02_admin_"+adminID.String()[:8]+"@example.com", pHash)
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Member User', $3, 'USER', true, false)", memberID, "ws02_member_"+memberID.String()[:8]+"@example.com", pHash)

		wsID := uuid.New()
		_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS02 Workspace', $2, 'WS02', $3)", wsID, "ws02-"+wsID.String()[:8], adminID)
		_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, adminID)
		_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'MEMBER')", wsID, memberID)

		itemID := uuid.New()
		_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, assignee_id, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Assigned Task', 'TODO', 'MEDIUM', $3, $4, $4)", itemID, wsID, memberID, adminID)

		tokenAdmin, _, _ := sessionStore.CreateSession(ctx, adminID, "127.0.0.1", "TestAgent")

		// Remove member
		reqDel := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s/members/%s", wsID, memberID), nil)
		reqDel.Header.Set("Origin", "http://example.com")
		reqDel.AddCookie(&http.Cookie{Name: "sid", Value: tokenAdmin})
		reqDel.AddCookie(&http.Cookie{Name: "__Host-sid", Value: tokenAdmin})
		wDel := httptest.NewRecorder()
		router.ServeHTTP(wDel, reqDel)

		if wDel.Code != http.StatusOK && wDel.Code != http.StatusNoContent {
			t.Fatalf("expected 200/204 on removing member, got %d. Body: %s", wDel.Code, wDel.Body.String())
		}

		// Verify assignee_id is NULL in DB
		var assigneeID *uuid.UUID
		err := pool.QueryRow(ctx, "SELECT assignee_id FROM work_items WHERE id = $1", itemID).Scan(&assigneeID)
		if err != nil {
			t.Fatalf("failed to query item: %v", err)
		}
		if assigneeID != nil {
			t.Errorf("expected assignee_id to be nullified after member removal, got %v", assigneeID)
		}

		// Removed member attempts to read workspace -> Must return 404/403 (No data leak)
		tokenMember, _, _ := sessionStore.CreateSession(ctx, memberID, "127.0.0.1", "TestAgent")
		reqRevokedRead := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID), nil)
		reqRevokedRead.AddCookie(&http.Cookie{Name: "sid", Value: tokenMember})
		reqRevokedRead.AddCookie(&http.Cookie{Name: "__Host-sid", Value: tokenMember})
		wRevokedRead := httptest.NewRecorder()
		router.ServeHTTP(wRevokedRead, reqRevokedRead)
		if wRevokedRead.Code != http.StatusNotFound && wRevokedRead.Code != http.StatusForbidden {
			t.Errorf("expected 404/403 for removed member accessing workspace, got %d", wRevokedRead.Code)
		}
	})

	// WS-03: Protection against removing last workspace ADMIN -> Returns 409 Conflict
	t.Run("WS-03_LastAdminProtection", func(t *testing.T) {
		adminID := uuid.New()
		pHash, _ := auth.HashPassword("Password123!")
		_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Solo Admin', $3, 'USER', true, false)", adminID, "ws03_admin_"+adminID.String()[:8]+"@example.com", pHash)

		wsID := uuid.New()
		_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS03 Workspace', $2, 'WS03', $3)", wsID, "ws03-"+wsID.String()[:8], adminID)
		_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, adminID)

		tokenAdmin, _, _ := sessionStore.CreateSession(ctx, adminID, "127.0.0.1", "TestAgent")

		// Attempt to remove sole admin
		reqDel := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s/members/%s", wsID, adminID), nil)
		reqDel.Header.Set("Origin", "http://example.com")
		reqDel.AddCookie(&http.Cookie{Name: "sid", Value: tokenAdmin})
		reqDel.AddCookie(&http.Cookie{Name: "__Host-sid", Value: tokenAdmin})
		wDel := httptest.NewRecorder()
		router.ServeHTTP(wDel, reqDel)

		if wDel.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict when removing last admin, got %d", wDel.Code)
		}
	})
}
