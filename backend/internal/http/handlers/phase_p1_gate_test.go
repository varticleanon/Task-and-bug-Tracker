package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
)

func TestPhase1GateSecurityVerification(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping P1 Gate integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	wsHandler := NewWorkspaceHandler(pool)
	itemHandler := NewItemHandler(pool)
	authMiddleware := middleware.RequireAuth(sessionStore, pool, false)
	requireAdminRole := middleware.RequireWorkspaceRole(pool, "ADMIN")
	requireMemberRole := middleware.RequireWorkspaceRole(pool, "MEMBER")

	// 1. Setup Active Member & Viewer users (must_change_password = false)
	memberID := uuid.New()
	passHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Gate Member', $3, 'USER', true, false)", memberID, "gate_member_"+memberID.String()[:8]+"@example.com", passHash)

	viewerID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Gate Viewer', $3, 'USER', true, false)", viewerID, "gate_viewer_"+viewerID.String()[:8]+"@example.com", passHash)

	tokenMember, _, _ := sessionStore.CreateSession(ctx, memberID, "127.0.0.1", "TestAgent")
	tokenViewer, _, _ := sessionStore.CreateSession(ctx, viewerID, "127.0.0.1", "TestAgent")

	// 2. Setup Workspace
	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'P1 Gate Workspace', $2, 'Gate Workspace', $3)", wsID, "gate-"+wsID.String()[:8], memberID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, memberID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'VIEWER')", wsID, viewerID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.With(requireAdminRole).Post("/members", wsHandler.AddMember)
		r.With(requireMemberRole).Post("/items", itemHandler.CreateItem)
	})

	// Test SEC-03: Member tries to access Super Admin global user management (Must return 403 Forbidden)
	rAdmin := chi.NewRouter()
	rAdmin.Use(authMiddleware, middleware.RequireSuperAdmin)
	rAdmin.Get("/api/v1/admin/users", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	reqSEC03 := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	reqSEC03.AddCookie(&http.Cookie{Name: "sid", Value: tokenMember})
	wSEC03 := httptest.NewRecorder()
	rAdmin.ServeHTTP(wSEC03, reqSEC03)
	if wSEC03.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for SEC-03 global admin access by regular member, got %d", wSEC03.Code)
	}

	// Test SEC-04: Viewer tries to create item (Must return 403 Forbidden)
	itemPayload, _ := json.Marshal(map[string]string{
		"type":  "TASK",
		"title": "Viewer Unauthorized Item",
	})
	reqSEC04 := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(itemPayload))
	reqSEC04.AddCookie(&http.Cookie{Name: "sid", Value: tokenViewer})
	wSEC04 := httptest.NewRecorder()
	r.ServeHTTP(wSEC04, reqSEC04)
	if wSEC04.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for SEC-04 viewer write action, got %d. Body: %s", wSEC04.Code, wSEC04.Body.String())
	}
}
