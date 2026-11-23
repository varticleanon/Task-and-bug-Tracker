package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
)

func TestItemSearchFilterSortingAndPagination(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping item search & filter test")
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
	passHash, _ := auth.HashPassword("Password123!")
	_, err = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Search Tester', $3, 'USER', true, false)", userID, "search_"+userID.String()[:8]+"@example.com", passHash)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	tokenUser, _, _ := sessionStore.CreateSession(ctx, userID, "127.0.0.1", "TestAgent")

	wsA := uuid.New()
	wsB := uuid.New()

	_, err = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Workspace Alpha', $2, 'Alpha WS', $3)", wsA, "ws-alpha-"+wsA.String()[:8], userID)
	if err != nil {
		t.Fatalf("failed to insert wsA: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsA, userID)
	if err != nil {
		t.Fatalf("failed to insert wsA member: %v", err)
	}

	_, err = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Workspace Beta', $2, 'Beta WS', $3)", wsB, "ws-beta-"+wsB.String()[:8], userID)
	if err != nil {
		t.Fatalf("failed to insert wsB: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsB, userID)
	if err != nil {
		t.Fatalf("failed to insert wsB member: %v", err)
	}

	now := time.Now().UTC()
	// Insert test data in Workspace A
	// Item 1: TASK, HIGH, TODO, unassigned
	item1ID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, description, status, priority, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, updated_at) VALUES ($1, $2, 'TASK', 'Fix Navigation Menu Bar', 'Responsive navigation menu glitch', 'TODO', 'HIGH', '2026-11-01', '', '', '', $3, $4, $5)", item1ID, wsA, userID, userID, now.Add(-3*time.Hour))
	if err != nil {
		t.Fatalf("failed to insert item1: %v", err)
	}

	// Item 2: BUG, CRITICAL, IN_PROGRESS, assigned to userID
	item2ID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, description, status, priority, assignee_id, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, updated_at) VALUES ($1, $2, 'BUG', 'Login Button Unresponsive', 'Clicking login does nothing', 'IN_PROGRESS', 'CRITICAL', $3, '2026-11-15', '1. Click login', 'Navigate home', 'Stuck on page', $4, $5, $6)", item2ID, wsA, userID, userID, userID, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("failed to insert item2: %v", err)
	}

	// Item 3: TASK, LOW, DONE, unassigned
	item3ID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, description, status, priority, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, updated_at) VALUES ($1, $2, 'TASK', 'Update Documentation Footer', 'Update copyright year to 2026', 'DONE', 'LOW', '2026-10-30', '', '', '', $3, $4, $5)", item3ID, wsA, userID, userID, now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("failed to insert item3: %v", err)
	}

	// Insert item in Workspace B with similar title to verify tenant isolation
	itemB1ID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, description, status, priority, steps_to_reproduce, expected_result, actual_result, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Fix Navigation Menu Bar in Beta', 'Beta menu issue', 'TODO', 'HIGH', '', '', '', $3, $4)", itemB1ID, wsB, userID, userID)
	if err != nil {
		t.Fatalf("failed to insert itemB1: %v", err)
	}

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Get("/items", itemHandler.ListItems)
	})

	// Test 1: Workspace Isolation and Default Sorting (updated_at DESC)
	reqList := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items", wsA.String()), nil)
	reqList.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list items, got %d. Body: %s", wList.Code, wList.Body.String())
	}

	var listResp struct {
		Data []struct {
			ID         string  `json:"id"`
			Title      string  `json:"title"`
			AssigneeID *string `json:"assignee_id"`
		} `json:"data"`
		Meta struct {
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(wList.Body.Bytes(), &listResp)

	if listResp.Meta.Total != 3 {
		t.Fatalf("expected total 3 items in workspace A, got %d", listResp.Meta.Total)
	}

	// First item should be item 3 because it has the newest updated_at
	if len(listResp.Data) > 0 && listResp.Data[0].ID != item3ID.String() {
		t.Fatalf("expected first item to be %s (newest updated_at), got %s", item3ID.String(), listResp.Data[0].ID)
	}

	// Test 2: Filter by type=BUG & priority=CRITICAL
	reqFilter := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items?type=BUG&priority=CRITICAL", wsA.String()), nil)
	reqFilter.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wFilter := httptest.NewRecorder()
	r.ServeHTTP(wFilter, reqFilter)

	_ = json.Unmarshal(wFilter.Body.Bytes(), &listResp)
	if listResp.Meta.Total != 1 || listResp.Data[0].ID != item2ID.String() {
		t.Fatalf("expected 1 BUG CRITICAL item (%s), got total %d", item2ID.String(), listResp.Meta.Total)
	}

	// Test 3: Filter by unassigned=true
	reqUnassigned := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items?unassigned=true", wsA.String()), nil)
	reqUnassigned.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wUnassigned := httptest.NewRecorder()
	r.ServeHTTP(wUnassigned, reqUnassigned)

	_ = json.Unmarshal(wUnassigned.Body.Bytes(), &listResp)
	if listResp.Meta.Total != 2 {
		t.Fatalf("expected 2 unassigned items, got %d. Data: %+v", listResp.Meta.Total, listResp.Data)
	}

	// Test 4: Parameterized Search query (matching title/description)
	reqSearch := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items?search=navigation", wsA.String()), nil)
	reqSearch.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wSearch := httptest.NewRecorder()
	r.ServeHTTP(wSearch, reqSearch)

	_ = json.Unmarshal(wSearch.Body.Bytes(), &listResp)
	if listResp.Meta.Total != 1 || listResp.Data[0].ID != item1ID.String() {
		t.Fatalf("expected 1 item matching search 'navigation' in workspace A, got total %d", listResp.Meta.Total)
	}

	// Test 5: Due Date bounds (due_before=2026-11-05)
	reqDue := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items?due_before=2026-11-05", wsA.String()), nil)
	reqDue.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wDue := httptest.NewRecorder()
	r.ServeHTTP(wDue, reqDue)

	_ = json.Unmarshal(wDue.Body.Bytes(), &listResp)
	if listResp.Meta.Total != 2 {
		t.Fatalf("expected 2 items due on/before 2026-11-05, got %d", listResp.Meta.Total)
	}
}
