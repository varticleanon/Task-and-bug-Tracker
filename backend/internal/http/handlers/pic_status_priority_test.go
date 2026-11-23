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

func TestPICMembershipStatusPriorityAndDueDateValidation(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping PIC and status validation test")
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

	activeMemberID := uuid.New()
	inactiveMemberID := uuid.New()
	nonMemberID := uuid.New()

	passHash, _ := auth.HashPassword("Password123!")

	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Active Member', $3, 'USER', true, false)", activeMemberID, "active_pic_"+activeMemberID.String()[:8]+"@example.com", passHash)
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Inactive Member', $3, 'USER', false, false)", inactiveMemberID, "inactive_pic_"+inactiveMemberID.String()[:8]+"@example.com", passHash)
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Non Member', $3, 'USER', true, false)", nonMemberID, "non_pic_"+nonMemberID.String()[:8]+"@example.com", passHash)

	tokenUser, _, _ := sessionStore.CreateSession(ctx, activeMemberID, "127.0.0.1", "TestAgent")

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'PIC Test Workspace', $2, 'Test', $3)", wsID, "pic-"+wsID.String()[:8], activeMemberID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, activeMemberID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'MEMBER')", wsID, inactiveMemberID)

	r := chi.NewRouter()
	r.Use(authMiddleware)
	r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
		r.Use(requireMemberRole)
		r.Post("/items", itemHandler.CreateItem)
		r.Patch("/items/{id}", itemHandler.UpdateItem)
	})

	// Test 1: Reject Inactive Member PIC -> 422 INVALID_PIC
	inactivePICPayload, _ := json.Marshal(map[string]interface{}{
		"type":        "TASK",
		"title":       "Task for Inactive User",
		"assignee_id": inactiveMemberID,
	})
	reqInactivePIC := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(inactivePICPayload))
	reqInactivePIC.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wInactivePIC := httptest.NewRecorder()
	r.ServeHTTP(wInactivePIC, reqInactivePIC)
	if wInactivePIC.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for inactive member PIC, got %d. Body: %s", wInactivePIC.Code, wInactivePIC.Body.String())
	}

	// Test 2: Reject Invalid Status enum -> 422 VALIDATION_ERROR
	invalidStatusPayload, _ := json.Marshal(map[string]interface{}{
		"type":   "TASK",
		"title":  "Invalid Status Task",
		"status": "UNKNOWN_STATUS",
	})
	reqInvalidStatus := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(invalidStatusPayload))
	reqInvalidStatus.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wInvalidStatus := httptest.NewRecorder()
	r.ServeHTTP(wInvalidStatus, reqInvalidStatus)
	if wInvalidStatus.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid status enum, got %d. Body: %s", wInvalidStatus.Code, wInvalidStatus.Body.String())
	}

	// Test 3: Reject Invalid Priority enum -> 422 VALIDATION_ERROR
	invalidPriorityPayload, _ := json.Marshal(map[string]interface{}{
		"type":     "TASK",
		"title":    "Invalid Priority Task",
		"priority": "URGENT",
	})
	reqInvalidPriority := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(invalidPriorityPayload))
	reqInvalidPriority.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wInvalidPriority := httptest.NewRecorder()
	r.ServeHTTP(wInvalidPriority, reqInvalidPriority)
	if wInvalidPriority.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid priority enum, got %d. Body: %s", wInvalidPriority.Code, wInvalidPriority.Body.String())
	}

	// Test 4: Reject Invalid Due Date format -> 422 VALIDATION_ERROR
	invalidDatePayload, _ := json.Marshal(map[string]interface{}{
		"type":     "TASK",
		"title":    "Invalid Due Date Task",
		"due_date": "15-11-2026",
	})
	reqInvalidDate := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(invalidDatePayload))
	reqInvalidDate.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wInvalidDate := httptest.NewRecorder()
	r.ServeHTTP(wInvalidDate, reqInvalidDate)
	if wInvalidDate.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid due date format, got %d. Body: %s", wInvalidDate.Code, wInvalidDate.Body.String())
	}

	// Test 5: Valid Creation & Audit Event Verification
	validPayload, _ := json.Marshal(map[string]interface{}{
		"type":        "TASK",
		"title":       "Valid Task with Audit",
		"status":      "IN_PROGRESS",
		"priority":    "CRITICAL",
		"assignee_id": activeMemberID,
		"due_date":    "2026-12-01",
	})
	reqValid := httptest.NewRequest("POST", "/api/v1/workspaces/"+wsID.String()+"/items", bytes.NewBuffer(validPayload))
	reqValid.AddCookie(&http.Cookie{Name: "sid", Value: tokenUser})
	wValid := httptest.NewRecorder()
	r.ServeHTTP(wValid, reqValid)
	if wValid.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for valid task creation, got %d. Body: %s", wValid.Code, wValid.Body.String())
	}

	var createResp map[string]map[string]interface{}
	_ = json.Unmarshal(wValid.Body.Bytes(), &createResp)
	itemIDStr := createResp["data"]["id"].(string)

	var auditCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events WHERE workspace_id = $1 AND entity_id = $2 AND action = 'CREATE_ITEM'", wsID, itemIDStr).Scan(&auditCount)
	if err != nil || auditCount != 1 {
		t.Fatalf("expected 1 CREATE_ITEM audit event, got count %d, err: %v", auditCount, err)
	}
}
