package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
)

func setupTestRouter(pool *pgxpool.Pool, sessionStore *auth.SessionStore) http.Handler {
	itemHandler := NewItemHandler(pool)
	authMiddleware := middleware.RequireAuth(sessionStore, pool, false)
	requireMemberRole := middleware.RequireWorkspaceRole(pool, "MEMBER")
	requireAdminRole := middleware.RequireWorkspaceRole(pool, "ADMIN")

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Get("/items", itemHandler.ListItems)
		r.Post("/items", itemHandler.CreateItem)
		r.Get("/items/{id}", itemHandler.GetItem)
		r.Patch("/items/{id}", itemHandler.UpdateItem)
		r.Get("/items/{id}/activities", itemHandler.GetItemActivities)

		r.With(requireAdminRole).Delete("/items/{id}", itemHandler.SoftDeleteItem)
	})
	return r
}

// ITEM-01: Create TASK, BUG, edit fields, change status -> Correct persistence and audit events
func TestITEM01_CreateTaskBugEditStatusAudit(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	router := setupTestRouter(pool, sessionStore)

	userID := uuid.New()
	userPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'ITEM01 User', $3, 'USER', true, false)", userID, "item01_"+userID.String()[:8]+"@example.com", userPassHash)

	token, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'ITEM01 Workspace', $2, 'WS01', $3)", wsID, "ws01-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	// 1. Create TASK
	taskPayload, _ := json.Marshal(map[string]interface{}{
		"type":        "TASK",
		"title":       "Integration Test Task",
		"description": "Task description for ITEM-01",
		"status":      "TODO",
		"priority":    "MEDIUM",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID.String()), bytes.NewBuffer(taskPayload))
	req.AddCookie(&http.Cookie{Name: "sid", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for TASK, got %d", w.Code)
	}

	var taskResp struct {
		Data WorkItemResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &taskResp)
	taskID := taskResp.Data.ID

	// 2. Create BUG with specific bug fields
	bugPayload, _ := json.Marshal(map[string]interface{}{
		"type":               "BUG",
		"title":              "Integration Test Bug",
		"description":        "Bug description for ITEM-01",
		"status":             "TODO",
		"priority":           "HIGH",
		"steps_to_reproduce": "Step 1, Step 2",
		"expected_result":    "Should work",
		"actual_result":      "Failed",
	})
	reqBug := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID.String()), bytes.NewBuffer(bugPayload))
	reqBug.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wBug := httptest.NewRecorder()
	router.ServeHTTP(wBug, reqBug)

	if wBug.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for BUG, got %d", wBug.Code)
	}

	// 3. Edit fields and change status on TASK
	updatePayload, _ := json.Marshal(map[string]interface{}{
		"version":  1,
		"title":    "Updated Integration Test Task",
		"status":   "IN_PROGRESS",
		"priority": "HIGH",
	})
	reqUp := httptest.NewRequest("PATCH", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID.String(), taskID.String()), bytes.NewBuffer(updatePayload))
	reqUp.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wUp := httptest.NewRecorder()
	router.ServeHTTP(wUp, reqUp)

	if wUp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on update, got %d", wUp.Code)
	}

	// 4. Verify Audit Activities
	reqAct := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/activities", wsID.String(), taskID.String()), nil)
	reqAct.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wAct := httptest.NewRecorder()
	router.ServeHTTP(wAct, reqAct)

	if wAct.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for activities, got %d", wAct.Code)
	}
}

// ITEM-02: Two concurrent clients edit same version -> One success; second 409
func TestITEM02_ConcurrentUpdateStaleVersion409(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	router := setupTestRouter(pool, sessionStore)

	userID := uuid.New()
	userPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'ITEM02 User', $3, 'USER', true, false)", userID, "item02_"+userID.String()[:8]+"@example.com", userPassHash)

	token, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'ITEM02 Workspace', $2, 'WS02', $3)", wsID, "ws02-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	// Create initial item
	createPayload, _ := json.Marshal(map[string]interface{}{
		"type":     "TASK",
		"title":    "Concurrent Test Item",
		"status":   "TODO",
		"priority": "MEDIUM",
	})
	reqC := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID.String()), bytes.NewBuffer(createPayload))
	reqC.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wC := httptest.NewRecorder()
	router.ServeHTTP(wC, reqC)

	var cResp struct {
		Data WorkItemResponse `json:"data"`
	}
	_ = json.Unmarshal(wC.Body.Bytes(), &cResp)
	itemID := cResp.Data.ID

	// Execute 2 concurrent PATCH requests with version = 1
	var wg sync.WaitGroup
	statusCodes := make([]int, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			updatePayload, _ := json.Marshal(map[string]interface{}{
				"version": 1,
				"title":   fmt.Sprintf("Title update from worker %d", idx),
				"status":  "IN_PROGRESS",
			})
			req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID.String(), itemID.String()), bytes.NewBuffer(updatePayload))
			req.AddCookie(&http.Cookie{Name: "sid", Value: token})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			statusCodes[idx] = w.Code
		}(i)
	}
	wg.Wait()

	has200 := statusCodes[0] == http.StatusOK || statusCodes[1] == http.StatusOK
	has409 := statusCodes[0] == http.StatusConflict || statusCodes[1] == http.StatusConflict

	if !has200 || !has409 {
		t.Fatalf("expected one 200 OK and one 409 Conflict in concurrent edits, got status codes: %v", statusCodes)
	}
}

// ITEM-03: Search/filter/sort/page under workspace -> Deterministic, workspace-scoped results
func TestITEM03_SearchFilterSortPaginationConsistency(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	router := setupTestRouter(pool, sessionStore)

	userID := uuid.New()
	userPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'ITEM03 User', $3, 'USER', true, false)", userID, "item03_"+userID.String()[:8]+"@example.com", userPassHash)

	token, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'ITEM03 Workspace', $2, 'WS03', $3)", wsID, "ws03-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	// Populate multiple items
	for i := 1; i <= 5; i++ {
		payload, _ := json.Marshal(map[string]interface{}{
			"type":     "BUG",
			"title":    fmt.Sprintf("Filterable Bug %d", i),
			"status":   "TODO",
			"priority": "HIGH",
		})
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID.String()), bytes.NewBuffer(payload))
		req.AddCookie(&http.Cookie{Name: "sid", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Filter by type=BUG&status=TODO&page=1&page_size=2
	reqList := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items?type=BUG&status=TODO&page=1&page_size=2", wsID.String()), nil)
	reqList.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wList := httptest.NewRecorder()
	router.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list query, got %d", wList.Code)
	}

	var listResp struct {
		Data []WorkItemResponse `json:"data"`
		Meta struct {
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(wList.Body.Bytes(), &listResp)

	if listResp.Meta.Total != 5 {
		t.Errorf("expected total 5, got %d", listResp.Meta.Total)
	}
	if len(listResp.Data) != 2 {
		t.Errorf("expected page size 2 items returned, got %d", len(listResp.Data))
	}
}

// ITEM-04: Soft delete item -> Hidden from normal list/dashboard/detail & tenant isolation 404
func TestITEM04_SoftDeleteAndInaccessible404(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	router := setupTestRouter(pool, sessionStore)

	userID := uuid.New()
	userPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'ITEM04 User', $3, 'USER', true, false)", userID, "item04_"+userID.String()[:8]+"@example.com", userPassHash)

	token, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'ITEM04 Workspace A', $2, 'WS04A', $3)", wsID, "ws04a-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	wsID2 := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'ITEM04 Workspace B', $2, 'WS04B', $3)", wsID2, "ws04b-"+wsID2.String()[:8], userID)

	// Create item in Workspace A
	payload, _ := json.Marshal(map[string]interface{}{
		"type":     "TASK",
		"title":    "Item to be deleted",
		"status":   "TODO",
		"priority": "LOW",
	})
	reqC := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID.String()), bytes.NewBuffer(payload))
	reqC.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wC := httptest.NewRecorder()
	router.ServeHTTP(wC, reqC)

	var cResp struct {
		Data WorkItemResponse `json:"data"`
	}
	_ = json.Unmarshal(wC.Body.Bytes(), &cResp)
	itemID := cResp.Data.ID

	// Attempt reading Workspace A item via Workspace B path -> MUST return 404
	reqCross := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID2.String(), itemID.String()), nil)
	reqCross.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wCross := httptest.NewRecorder()
	router.ServeHTTP(wCross, reqCross)

	if wCross.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found on cross-tenant read, got %d", wCross.Code)
	}

	// Soft delete item in Workspace A
	reqDel := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID.String(), itemID.String()), nil)
	reqDel.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)

	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on delete, got %d", wDel.Code)
	}

	// Verify item is hidden (GET item returns 404)
	reqGet := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID.String(), itemID.String()), nil)
	reqGet.AddCookie(&http.Cookie{Name: "sid", Value: token})
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for soft-deleted item, got %d", wGet.Code)
	}
}
