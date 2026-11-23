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

func TestDashboardAggregationsAndIsolation(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping dashboard test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	dashHandler := NewDashboardHandler(pool)
	authMiddleware := middleware.RequireAuth(sessionStore, pool, false)
	requireMemberRole := middleware.RequireWorkspaceRole(pool, "MEMBER")

	// 1. Setup User A and Workspace A
	userAID := uuid.New()
	userAPassHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Dash User A', $3, 'USER', true, false)", userAID, "dash_a_"+userAID.String()[:8]+"@example.com", userAPassHash)

	tokenUserA, _, _ := sessionStore.CreateSession(ctx, userAID, "127.0.0.1", "TestAgent")

	wsAID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'Dashboard Workspace A', $2, 'WS A', $3)", wsAID, "dasha-"+wsAID.String()[:8], userAID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsAID, userAID)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	// Insert items in Workspace A:
	// Item 1: TASK, TODO, MEDIUM, assigned to User A, due yesterday (OVERDUE)
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, assignee_id, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Task 1', 'TODO', 'MEDIUM', $3, $4, '', '', '', $3, $3)", uuid.New(), wsAID, userAID, yesterday)

	// Item 2: BUG, IN_PROGRESS, CRITICAL, unassigned, due tomorrow
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by) VALUES ($1, $2, 'BUG', 'Bug 2', 'IN_PROGRESS', 'CRITICAL', $3, '', '', '', $4, $4)", uuid.New(), wsAID, tomorrow, userAID)

	// Item 3: BUG, DONE, LOW, assigned to User A, due yesterday (Not overdue because status is DONE)
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, assignee_id, due_date, steps_to_reproduce, expected_result, actual_result, created_by, updated_by) VALUES ($1, $2, 'BUG', 'Bug 3', 'DONE', 'LOW', $3, $4, '', '', '', $3, $3)", uuid.New(), wsAID, userAID, yesterday)

	// Item 4: TASK, BLOCKED, HIGH, unassigned
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, steps_to_reproduce, expected_result, actual_result, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Task 4', 'BLOCKED', 'HIGH', '', '', '', $3, $3)", uuid.New(), wsAID, userAID)

	// Item 5 (Soft deleted): TASK, TODO, HIGH - Should not be counted in dashboard
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, steps_to_reproduce, expected_result, actual_result, created_by, updated_by, deleted_at) VALUES ($1, $2, 'TASK', 'Deleted Task', 'TODO', 'HIGH', '', '', '', $3, $3, NOW())", uuid.New(), wsAID, userAID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Get("/dashboard", dashHandler.GetDashboardSummary)
	})

	reqDash := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/dashboard", wsAID.String()), nil)
	reqDash.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	wDash := httptest.NewRecorder()

	r.ServeHTTP(wDash, reqDash)

	if wDash.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for dashboard, got %d. Body: %s", wDash.Code, wDash.Body.String())
	}

	var resp struct {
		Data DashboardResponse `json:"data"`
	}
	_ = json.Unmarshal(wDash.Body.Bytes(), &resp)

	d := resp.Data
	if d.TotalOpen != 3 {
		t.Errorf("expected total_open 3, got %d", d.TotalOpen)
	}
	if d.Todo != 1 {
		t.Errorf("expected todo 1, got %d", d.Todo)
	}
	if d.InProgress != 1 {
		t.Errorf("expected in_progress 1, got %d", d.InProgress)
	}
	if d.Blocked != 1 {
		t.Errorf("expected blocked 1, got %d", d.Blocked)
	}
	if d.Done != 1 {
		t.Errorf("expected done 1, got %d", d.Done)
	}
	if d.Bugs != 2 {
		t.Errorf("expected bugs 2, got %d", d.Bugs)
	}
	if d.Tasks != 2 {
		t.Errorf("expected tasks 2, got %d", d.Tasks)
	}
	if d.AssignedToMe != 2 {
		t.Errorf("expected assigned_to_me 2, got %d", d.AssignedToMe)
	}
	if d.Overdue != 1 {
		t.Errorf("expected overdue 1 (only open items), got %d", d.Overdue)
	}
}
