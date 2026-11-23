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

func TestWorkspaceManagementAndLastAdminGuard(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping workspace integration test")
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
	superAdminMiddleware := middleware.RequireSuperAdmin

	// 1. Setup Super Admin User
	saID := uuid.New()
	saEmail := "ws_sa_" + saID.String()[:8] + "@example.com"
	saPassHash, _ := auth.HashPassword("SuperAdminPass123!")
	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, $3, $4, 'SUPER_ADMIN', true, false)",
		saID, saEmail, "Workspace Super Admin", saPassHash,
	)
	if err != nil {
		t.Fatalf("failed to insert test super admin: %v", err)
	}

	tokenSA, _, err := sessionStore.CreateSession(ctx, saID, "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// 2. Test Super Admin Creates Workspace (POST /workspaces)
	wsSlug := "ws-" + uuid.New().String()[:8]
	createWsPayload, _ := json.Marshal(map[string]string{
		"name":        "Test Workspace",
		"slug":        wsSlug,
		"description": "Integration test workspace",
	})
	reqCreateWs := httptest.NewRequest("POST", "/api/v1/workspaces", bytes.NewBuffer(createWsPayload))
	reqCreateWs.AddCookie(&http.Cookie{Name: "sid", Value: tokenSA})
	wCreateWs := httptest.NewRecorder()

	protectedCreateWs := authMiddleware(superAdminMiddleware(http.HandlerFunc(wsHandler.CreateWorkspace)))
	protectedCreateWs.ServeHTTP(wCreateWs, reqCreateWs)

	if wCreateWs.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for workspace creation, got %d. Body: %s", wCreateWs.Code, wCreateWs.Body.String())
	}

	var wsResp map[string]map[string]interface{}
	_ = json.Unmarshal(wCreateWs.Body.Bytes(), &wsResp)
	wsIDStr := wsResp["data"]["id"].(string)
	wsID, _ := uuid.Parse(wsIDStr)

	// 3. Test Archive Workspace (POST /workspaces/{id}/archive)
	r := chi.NewRouter()
	r.Use(authMiddleware, superAdminMiddleware)
	r.Post("/api/v1/workspaces/{workspaceId}/archive", wsHandler.ArchiveWorkspace)
	r.Post("/api/v1/workspaces/{workspaceId}/restore", wsHandler.RestoreWorkspace)

	reqArchive := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/archive", nil)
	reqArchive.AddCookie(&http.Cookie{Name: "sid", Value: tokenSA})
	wArchive := httptest.NewRecorder()
	r.ServeHTTP(wArchive, reqArchive)

	if wArchive.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for archiving workspace, got %d. Body: %s", wArchive.Code, wArchive.Body.String())
	}

	// 4. Test Restore Workspace (POST /workspaces/{id}/restore)
	reqRestore := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/restore", nil)
	reqRestore.AddCookie(&http.Cookie{Name: "sid", Value: tokenSA})
	wRestore := httptest.NewRecorder()
	r.ServeHTTP(wRestore, reqRestore)

	if wRestore.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for restoring workspace, got %d. Body: %s", wRestore.Code, wRestore.Body.String())
	}

	// 5. Test Last Workspace ADMIN Guard (Cannot remove sole workspace ADMIN)
	rMembers := chi.NewRouter()
	rMembers.Use(authMiddleware)
	rMembers.Delete("/api/v1/workspaces/{workspaceId}/members/{userId}", wsHandler.RemoveMember)

	reqRemoveSoleAdmin := httptest.NewRequest("DELETE", "/api/v1/workspaces/"+wsID.String()+"/members/"+saID.String(), nil)
	reqRemoveSoleAdmin.AddCookie(&http.Cookie{Name: "sid", Value: tokenSA})
	wRemoveSoleAdmin := httptest.NewRecorder()

	rMembers.ServeHTTP(wRemoveSoleAdmin, reqRemoveSoleAdmin)

	if wRemoveSoleAdmin.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when removing sole workspace ADMIN, got %d. Body: %s", wRemoveSoleAdmin.Code, wRemoveSoleAdmin.Body.String())
	}
}
