package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestItemActivityTimelineAndAuditLogging(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping item activities test")
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

	userID := uuid.New()
	userPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Audit Test User', $3, 'USER', true, false)", userID, "audit_user_"+userID.String()[:8]+"@example.com", userPassHash)

	tokenUser, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Audit Workspace', $2, 'WS Audit', $3)", wsID, "audws-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Post("/items", itemHandler.CreateItem)
		r.Patch("/items/{id}", itemHandler.UpdateItem)
		r.Get("/items/{id}/activities", itemHandler.GetItemActivities)
	})

	createPayload, _ := json.Marshal(map[string]interface{}{
		"type":        "TASK",
		"title":       "Audit Logging Task",
		"description": "Test audit trail completeness",
		"status":      "TODO",
		"priority":    "MEDIUM",
		"assignee_id": userID,
	})
	reqCreate := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID.String()), bytes.NewBuffer(createPayload))
	reqCreate.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wCreate := httptest.NewRecorder()
	r.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on item creation, got %d. Body: %s", wCreate.Code, wCreate.Body.String())
	}

	var createResp struct {
		Data WorkItemResponse `json:"data"`
	}
	_ = json.Unmarshal(wCreate.Body.Bytes(), &createResp)
	itemID := createResp.Data.ID

	updatePayload, _ := json.Marshal(map[string]interface{}{
		"version":  1,
		"status":   "IN_PROGRESS",
		"priority": "HIGH",
	})
	reqUpdate := httptest.NewRequest("PATCH", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID.String(), itemID.String()), bytes.NewBuffer(updatePayload))
	reqUpdate.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wUpdate := httptest.NewRecorder()
	r.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on item update, got %d. Body: %s", wUpdate.Code, wUpdate.Body.String())
	}

	reqAct := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/activities", wsID.String(), itemID.String()), nil)
	reqAct.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wAct := httptest.NewRecorder()
	r.ServeHTTP(wAct, reqAct)

	if wAct.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on activities fetch, got %d. Body: %s", wAct.Code, wAct.Body.String())
	}

	var actResp struct {
		Data []ActivityResponse `json:"data"`
	}
	_ = json.Unmarshal(wAct.Body.Bytes(), &actResp)

	if len(actResp.Data) < 2 {
		t.Fatalf("expected at least 2 audit events, got %d", len(actResp.Data))
	}

	if actResp.Data[0].Action != "UPDATE_ITEM" {
		t.Errorf("expected latest action to be UPDATE_ITEM, got %s", actResp.Data[0].Action)
	}
	if actResp.Data[1].Action != "CREATE_ITEM" {
		t.Errorf("expected earlier action to be CREATE_ITEM, got %s", actResp.Data[1].Action)
	}
	if actResp.Data[0].ActorName != "Audit Test User" {
		t.Errorf("expected actor_name 'Audit Test User', got '%s'", actResp.Data[0].ActorName)
	}

	detailsStr := string(actResp.Data[0].Details)
	if bytes.Contains([]byte(detailsStr), []byte("password")) || bytes.Contains([]byte(detailsStr), []byte("secret")) {
		t.Errorf("sensitive information found in audit event details: %s", detailsStr)
	}
}
