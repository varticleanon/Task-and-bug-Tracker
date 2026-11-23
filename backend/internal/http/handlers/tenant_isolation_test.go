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

func TestTenantIsolationAndIDORProtections(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping tenant isolation test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	itemHandler := NewItemHandler(pool)
	authMiddleware := middleware.RequireAuth(sessionStore, pool, false)
	requireMemberRole := middleware.RequireWorkspaceRole(pool, "MEMBER")

	// 1. Setup User A (Member of Workspace A) and User B (Member of Workspace B)
	userAID := uuid.New()
	userAPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'User A', $3, 'USER', true, false)", userAID, "usera_"+userAID.String()[:8]+"@example.com", userAPassHash)

	userBID := uuid.New()
	userBPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'User B', $3, 'USER', true, false)", userBID, "userb_"+userBID.String()[:8]+"@example.com", userBPassHash)

	tokenUserA, _, _ := sessionStore.CreateSession(ctx, userAID, "127.0.0.1", "TestAgent")

	// 2. Setup Workspace A and Workspace B
	wsAID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Workspace A', $2, 'Tenant A', $3)", wsAID, "wsa-"+wsAID.String()[:8], userAID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsAID, userAID)

	wsBID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Workspace B', $2, 'Tenant B', $3)", wsBID, "wsb-"+wsBID.String()[:8], userBID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsBID, userBID)

	// 3. Create Item B in Workspace B
	itemBID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by, version) VALUES ($1, $2, 'TASK', 'Secret Item B', 'TODO', 'HIGH', $3, $3, 1)", itemBID, wsBID, userBID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Get("/items/{id}", itemHandler.GetItem)
		r.Patch("/items/{id}", itemHandler.UpdateItem)
		r.Delete("/items/{id}", itemHandler.SoftDeleteItem)
	})

	// 4. Test SEC-01: User A requests Item B via Workspace B's URL -> Must return 404 Not Found
	reqSEC01 := httptest.NewRequest("GET", "/api/v1/workspaces/"+wsBID.String()+"/items/"+itemBID.String(), nil)
	reqSEC01.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	wSEC01 := httptest.NewRecorder()

	r.ServeHTTP(wSEC01, reqSEC01)
	if wSEC01.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for SEC-01 cross-tenant request, got %d. Body: %s", wSEC01.Code, wSEC01.Body.String())
	}

	// 5. Test SEC-02: User A changes path workspaceId to Workspace A but keeps Item B ID -> Must return 404 Not Found
	reqSEC02 := httptest.NewRequest("GET", "/api/v1/workspaces/"+wsAID.String()+"/items/"+itemBID.String(), nil)
	reqSEC02.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	wSEC02 := httptest.NewRecorder()

	r.ServeHTTP(wSEC02, reqSEC02)
	if wSEC02.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for SEC-02 tenant ID swapping, got %d. Body: %s", wSEC02.Code, wSEC02.Body.String())
	}

	// 6. Test SEC-02 (Mutation): User A attempts PATCH on Item B via Workspace A path -> Must return 404 Not Found
	patchPayload, _ := json.Marshal(map[string]interface{}{
		"version": 1,
		"title":   "Hacked Title",
	})
	reqSEC02Patch := httptest.NewRequest("PATCH", "/api/v1/workspaces/"+wsAID.String()+"/items/"+itemBID.String(), bytes.NewBuffer(patchPayload))
	reqSEC02Patch.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	wSEC02Patch := httptest.NewRecorder()

	r.ServeHTTP(wSEC02Patch, reqSEC02Patch)
	if wSEC02Patch.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when mutating cross-tenant item, got %d. Body: %s", wSEC02Patch.Code, wSEC02Patch.Body.String())
	}

	// 7. Verify Item B was NOT modified in Workspace B
	var currentTitle string
	_ = pool.QueryRow(ctx, "SELECT title FROM work_items WHERE id = $1 AND workspace_id = $2", itemBID, wsBID).Scan(&currentTitle)
	if currentTitle != "Secret Item B" {
		t.Fatalf("expected Item B title to remain 'Secret Item B', got '%s'", currentTitle)
	}
}
