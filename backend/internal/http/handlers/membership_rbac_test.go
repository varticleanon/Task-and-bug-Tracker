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

func TestMembershipRBACAndAtomicUnassign(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping membership RBAC test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	wsHandler := NewWorkspaceHandler(pool)
	authMiddleware := middleware.RequireAuth(sessionStore, pool, false)
	requireAdminRole := middleware.RequireWorkspaceRole(pool, "ADMIN")

	// 1. Setup Admin and Member Users with must_change_password = false
	adminID := uuid.New()
	adminPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Admin User', $3, 'USER', true, false)",
		adminID, "admin_"+adminID.String()[:8]+"@example.com", adminPassHash,
	)

	memberID := uuid.New()
	memberEmail := "member_" + memberID.String()[:8] + "@example.com"
	_, _ = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Member User', $3, 'USER', true, false)",
		memberID, memberEmail, adminPassHash,
	)

	tokenAdmin, _, _ := sessionStore.CreateSession(ctx, adminID, "127.0.0.1", "TestAgent")
	tokenMember, _, _ := sessionStore.CreateSession(ctx, memberID, "127.0.0.1", "TestAgent")

	// 2. Setup Workspace
	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'RBAC Test Workspace', $2, 'Test', $3)", wsID, "rbac-"+wsID.String()[:8], adminID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, adminID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'MEMBER')", wsID, memberID)

	// 3. Setup Item assigned to Member
	itemID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, assignee_id, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Assignee Test Item', 'TODO', 'MEDIUM', $3, $4, $4)", itemID, wsID, memberID, adminID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireAdminRole)
		r.Post("/members", wsHandler.AddMember)
		r.Patch("/members/{userId}", wsHandler.UpdateMemberRole)
		r.Delete("/members/{userId}", wsHandler.RemoveMember)
	})

	// 4. Test MEMBER tries to change role (Must return 403 Forbidden)
	rolePayload, _ := json.Marshal(map[string]string{"role": "ADMIN"})
	reqMemberForbidden := httptest.NewRequest("PATCH", "/api/v1/workspaces/"+wsID.String()+"/members/"+memberID.String(), bytes.NewBuffer(rolePayload))
	reqMemberForbidden.AddCookie(&http.Cookie{Name: "sid", Value: tokenMember})
	wMemberForbidden := httptest.NewRecorder()

	r.ServeHTTP(wMemberForbidden, reqMemberForbidden)
	if wMemberForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when MEMBER attempts to update role, got %d. Body: %s", wMemberForbidden.Code, wMemberForbidden.Body.String())
	}

	// 5. Test Demoting sole ADMIN (Must return 409 Conflict - WS-03)
	demoteAdminPayload, _ := json.Marshal(map[string]string{"role": "MEMBER"})
	reqDemoteAdmin := httptest.NewRequest("PATCH", "/api/v1/workspaces/"+wsID.String()+"/members/"+adminID.String(), bytes.NewBuffer(demoteAdminPayload))
	reqDemoteAdmin.AddCookie(&http.Cookie{Name: "sid", Value: tokenAdmin})
	wDemoteAdmin := httptest.NewRecorder()

	r.ServeHTTP(wDemoteAdmin, reqDemoteAdmin)
	if wDemoteAdmin.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when demoting last ADMIN, got %d. Body: %s", wDemoteAdmin.Code, wDemoteAdmin.Body.String())
	}

	// 6. Test Removing Member atomically unassigns item PIC (WS-02)
	reqRemoveMember := httptest.NewRequest("DELETE", "/api/v1/workspaces/"+wsID.String()+"/members/"+memberID.String(), nil)
	reqRemoveMember.AddCookie(&http.Cookie{Name: "sid", Value: tokenAdmin})
	wRemoveMember := httptest.NewRecorder()

	r.ServeHTTP(wRemoveMember, reqRemoveMember)
	if wRemoveMember.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on member removal, got %d. Body: %s", wRemoveMember.Code, wRemoveMember.Body.String())
	}

	// Verify assignee_id IS NULL in DB
	var assigneeID *uuid.UUID
	err = pool.QueryRow(ctx, "SELECT assignee_id FROM work_items WHERE id = $1", itemID).Scan(&assigneeID)
	if err != nil {
		t.Fatalf("failed to query item assignee after member removal: %v", err)
	}
	if assigneeID != nil {
		t.Fatalf("expected assignee_id to be NULL after member removal, got %v", assigneeID)
	}
}
