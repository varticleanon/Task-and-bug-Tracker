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

func TestRESTWorkItemCRUDValidationAndLifecycle(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping REST item CRUD test")
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

	// 1. Setup Active User & Non-Member User
	userID := uuid.New()
	passHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'CRUD Member', $3, 'USER', true, false)", userID, "crud_member_"+userID.String()[:8]+"@example.com", passHash)

	nonMemberID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Non Member', $3, 'USER', true, false)", nonMemberID, "non_member_"+nonMemberID.String()[:8]+"@example.com", passHash)

	tokenUser, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'CRUD Test Workspace', $2, 'Test', $3)", wsID, "crud-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Get("/items", itemHandler.ListItems)
		r.Post("/items", itemHandler.CreateItem)
		r.Get("/items/{id}", itemHandler.GetItem)
		r.Patch("/items/{id}", itemHandler.UpdateItem)
		r.Delete("/items/{id}", itemHandler.SoftDeleteItem)
	})

	// Validation Test 1: Short title (< 3 chars) -> 422 Unprocessable Entity
	shortTitlePayload, _ := json.Marshal(map[string]interface{}{
		"type":  "TASK",
		"title": "Hi",
	})
	reqShort := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(shortTitlePayload))
	reqShort.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wShort := httptest.NewRecorder()
	r.ServeHTTP(wShort, reqShort)
	if wShort.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for title < 3 chars, got %d", wShort.Code)
	}

	// Validation Test 2: Invalid PIC (Non-member assigned as PIC) -> 422 Unprocessable Entity (WS-04)
	invalidPICPayload, _ := json.Marshal(map[string]interface{}{
		"type":        "TASK",
		"title":       "Task with Invalid PIC",
		"assignee_id": nonMemberID,
	})
	reqPIC := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(invalidPICPayload))
	reqPIC.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wPIC := httptest.NewRecorder()
	r.ServeHTTP(wPIC, reqPIC)
	if wPIC.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity for invalid non-member PIC, got %d. Body: %s", wPIC.Code, wPIC.Body.String())
	}

	// CRUD Test 3: Create valid TASK
	createPayload, _ := json.Marshal(map[string]interface{}{
		"type":        "TASK",
		"title":       "Integrate REST endpoints",
		"description": "Implement CRUD logic and validations",
		"status":      "TODO",
		"priority":    "HIGH",
		"assignee_id": userID,
		"due_date":    "2026-11-15",
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(createPayload))
	reqCreate.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wCreate := httptest.NewRecorder()
	r.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on valid TASK creation, got %d. Body: %s", wCreate.Code, wCreate.Body.String())
	}

	var createdResp map[string]map[string]interface{}
	_ = json.Unmarshal(wCreate.Body.Bytes(), &createdResp)
	itemID := createdResp["data"]["id"].(string)

	// CRUD Test 4: Get item detail
	reqGet := httptest.NewRequest("GET", "/api/v1/workspaces/"+wsID.String()+"/items/"+itemID, nil)
	reqGet.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET item detail, got %d", wGet.Code)
	}

	// CRUD Test 5: List items with pagination envelope
	reqList := httptest.NewRequest("GET", "/api/v1/workspaces/"+wsID.String()+"/items?page=1&page_size=10", nil)
	reqList.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on List items, got %d", wList.Code)
	}

	// CRUD Test 6: Soft Delete item -> 204 No Content
	reqDelete := httptest.NewRequest("DELETE", "/api/v1/workspaces/"+wsID.String()+"/items/"+itemID, nil)
	reqDelete.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wDelete := httptest.NewRecorder()
	r.ServeHTTP(wDelete, reqDelete)
	if wDelete.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on Soft Delete, got %d", wDelete.Code)
	}
}
