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

func TestWorkItemDomainModelAndVersioning(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping domain model test")
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

	// 1. Setup User & Workspace
	userID := uuid.New()
	userPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Domain User', $3, 'USER', true, false)", userID, "domain_user_"+userID.String()[:8]+"@example.com", userPassHash)

	tokenUser, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Domain Test Workspace', $2, 'Test', $3)", wsID, "domain-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Post("/items", itemHandler.CreateItem)
		r.Get("/items/{id}", itemHandler.GetItem)
		r.Patch("/items/{id}", itemHandler.UpdateItem)
		r.Delete("/items/{id}", itemHandler.SoftDeleteItem)
	})

	// 2. Test ITEM-01: Create BUG item with bug fields
	bugPayload, _ := json.Marshal(map[string]interface{}{
		"type":               "BUG",
		"title":              "Crash on checkout page",
		"description":        "App crashes when clicking pay button",
		"status":             "TODO",
		"priority":           "CRITICAL",
		"assignee_id":        userID,
		"steps_to_reproduce": "1. Add item 2. Click checkout 3. Click pay",
		"expected_result":    "Payment successful",
		"actual_result":      "Null pointer exception",
	})
	reqCreateBug := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(bugPayload))
	reqCreateBug.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wCreateBug := httptest.NewRecorder()

	r.ServeHTTP(wCreateBug, reqCreateBug)
	if wCreateBug.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on BUG creation, got %d. Body: %s", wCreateBug.Code, wCreateBug.Body.String())
	}

	var createResp map[string]map[string]interface{}
	_ = json.Unmarshal(wCreateBug.Body.Bytes(), &createResp)
	item := createResp["data"]
	itemIDStr := item["id"].(string)
	version := int(item["version"].(float64))

	if version != 1 {
		t.Fatalf("expected initial version to be 1, got %d", version)
	}

	// 3. Test Valid Update (version 1 -> 2)
	validUpdatePayload, _ := json.Marshal(map[string]interface{}{
		"version":  version,
		"status":   "IN_PROGRESS",
		"priority": "HIGH",
	})
	reqUpdate := httptest.NewRequest("PATCH", "/api/v1/workspaces/"+wsID.String()+"/items/"+itemIDStr, bytes.NewBuffer(validUpdatePayload))
	reqUpdate.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wUpdate := httptest.NewRecorder()

	r.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on valid update, got %d. Body: %s", wUpdate.Code, wUpdate.Body.String())
	}

	var updateResp map[string]map[string]interface{}
	_ = json.Unmarshal(wUpdate.Body.Bytes(), &updateResp)
	newVersion := int(updateResp["data"]["version"].(float64))
	if newVersion != 2 {
		t.Fatalf("expected updated version to be 2, got %d", newVersion)
	}

	// 4. Test ITEM-02: Stale update (using version 1 instead of 2) -> Must return 409 Conflict
	staleUpdatePayload, _ := json.Marshal(map[string]interface{}{
		"version": 1,
		"status":  "DONE",
	})
	reqStale := httptest.NewRequest("PATCH", "/api/v1/workspaces/"+wsID.String()+"/items/"+itemIDStr, bytes.NewBuffer(staleUpdatePayload))
	reqStale.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wStale := httptest.NewRecorder()

	r.ServeHTTP(wStale, reqStale)
	if wStale.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on stale update, got %d. Body: %s", wStale.Code, wStale.Body.String())
	}

	// 5. Test Type Transition BUG -> TASK clears bug fields
	transitionPayload, _ := json.Marshal(map[string]interface{}{
		"version": newVersion,
		"type":    "TASK",
	})
	reqTransition := httptest.NewRequest("PATCH", "/api/v1/workspaces/"+wsID.String()+"/items/"+itemIDStr, bytes.NewBuffer(transitionPayload))
	reqTransition.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wTransition := httptest.NewRecorder()

	r.ServeHTTP(wTransition, reqTransition)
	if wTransition.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on type transition BUG -> TASK, got %d. Body: %s", wTransition.Code, wTransition.Body.String())
	}

	var transitionResp map[string]map[string]interface{}
	_ = json.Unmarshal(wTransition.Body.Bytes(), &transitionResp)
	transItem := transitionResp["data"]
	if transItem["type"] != "TASK" {
		t.Fatalf("expected type to be TASK, got %s", transItem["type"])
	}
	if steps, ok := transItem["steps_to_reproduce"]; ok && steps != "" && steps != nil {
		t.Fatalf("expected steps_to_reproduce to be cleared when transitioning to TASK, got %v", steps)
	}

	// 6. Test ITEM-04: Soft Delete Work Item
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/workspaces/"+wsID.String()+"/items/"+itemIDStr, nil)
	reqDelete.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wDelete := httptest.NewRecorder()

	r.ServeHTTP(wDelete, reqDelete)
	if wDelete.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on soft delete, got %d. Body: %s", wDelete.Code, wDelete.Body.String())
	}

	// 7. Verify GET on soft-deleted item returns 404 Not Found
	reqGetDeleted := httptest.NewRequest("GET", "/api/v1/workspaces/"+wsID.String()+"/items/"+itemIDStr, nil)
	reqGetDeleted.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wGetDeleted := httptest.NewRecorder()

	r.ServeHTTP(wGetDeleted, reqGetDeleted)
	if wGetDeleted.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when requesting soft-deleted item, got %d. Body: %s", wGetDeleted.Code, wGetDeleted.Body.String())
	}
}
