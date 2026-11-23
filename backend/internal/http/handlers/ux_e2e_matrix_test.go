package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/storage"
)

func setupE2ERouter(pool *pgxpool.Pool, sessionStore *auth.SessionStore, store storage.Storage) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.OriginValidation)

	authHandler := NewAuthHandler(pool, sessionStore, []byte("super-secret-key-for-dev-only"), false)
	itemHandler := NewItemHandler(pool)
	attachHandler := NewAttachmentHandler(pool, store)

	r.Group(func(r chi.Router) {
		r.Post("/api/v1/auth/login", authHandler.Login)
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(sessionStore, pool, false))
		r.Get("/api/v1/auth/me", authHandler.Me)

		r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRole(pool, "VIEWER"))

			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Post("/items", itemHandler.CreateItem)
			r.Get("/items/{id}", itemHandler.GetItem)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Patch("/items/{id}", itemHandler.UpdateItem)

			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Post("/items/{id}/attachments", attachHandler.UploadAttachment)
			r.Get("/items/{id}/attachments/{aid}/content", attachHandler.GetAttachmentContent)
		})
	})
	return r
}

func TestUX01_EndToEndUserFlow(t *testing.T) {
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
	tmpDir, _ := os.MkdirTemp("", "e2e_test_*")
	defer os.RemoveAll(tmpDir)
	store, _ := storage.NewLocalStorage(tmpDir)

	router := setupE2ERouter(pool, sessionStore, store)

	// Step 1: Seed Active User & Workspace
	userEmail := "e2e_user_" + uuid.New().String()[:8] + "@example.com"
	rawPassword := "Password123!"
	passHash, _ := auth.HashPassword(rawPassword)

	userID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'E2E Tester', $3, 'USER', true, false)", userID, userEmail, passHash)

	picID := uuid.New()
	picEmail := "e2e_pic_" + uuid.New().String()[:8] + "@example.com"
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'E2E PIC', $3, 'USER', true, false)", picID, picEmail, passHash)

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'E2E Workspace', $2, 'Test WS', $3)", wsID, "e2e-ws-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'MEMBER')", wsID, picID)

	// Step 2: Login via /auth/login
	loginPayload := map[string]string{
		"email":    userEmail,
		"password": rawPassword,
	}
	loginBody, _ := json.Marshal(loginPayload)
	reqLogin := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	reqLogin.Header.Set("Origin", "http://example.com")
	wLogin := httptest.NewRecorder()

	router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("E2E Step 2 Login failed: expected 200 OK, got %d. Body: %s", wLogin.Code, wLogin.Body.String())
	}

	loginCookies := wLogin.Result().Cookies()
	if len(loginCookies) == 0 {
		t.Fatalf("E2E Step 2 Login failed: no session cookie returned")
	}

	attachAuthCookies := func(req *http.Request) {
		for _, c := range loginCookies {
			req.AddCookie(c)
			req.AddCookie(&http.Cookie{Name: "sid", Value: c.Value})
			req.AddCookie(&http.Cookie{Name: "__Host-sid", Value: c.Value})
		}
	}

	// Step 3: Create BUG Item
	bugPayload := map[string]interface{}{
		"type":               "BUG",
		"title":              "E2E Login Response Lag",
		"description":        "Tombol login mengalami latency 2 detik",
		"status":             "TODO",
		"priority":           "HIGH",
		"steps_to_reproduce": "1. Masukkan email & pass\n2. Klik Login",
		"expected_result":    "Navigasi instan < 300ms",
		"actual_result":      "Spinner berputar 2 detik",
	}
	bugBody, _ := json.Marshal(bugPayload)
	reqBug := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID), bytes.NewBuffer(bugBody))
	reqBug.Header.Set("Content-Type", "application/json")
	reqBug.Header.Set("Origin", "http://example.com")
	attachAuthCookies(reqBug)
	wBug := httptest.NewRecorder()

	router.ServeHTTP(wBug, reqBug)
	if wBug.Code != http.StatusCreated {
		t.Fatalf("E2E Step 3 Create BUG failed: expected 201 Created, got %d. Body: %s", wBug.Code, wBug.Body.String())
	}

	var bugResp struct {
		Data struct {
			ID      uuid.UUID `json:"id"`
			Version int       `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wBug.Body.Bytes(), &bugResp)
	itemID := bugResp.Data.ID
	currentVersion := bugResp.Data.Version

	// Step 4: Assign PIC & Update Status to IN_PROGRESS
	patchPayload := map[string]interface{}{
		"version":     currentVersion,
		"assignee_id": picID,
		"status":      "IN_PROGRESS",
	}
	patchBody, _ := json.Marshal(patchPayload)
	reqPatch := httptest.NewRequest("PATCH", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID, itemID), bytes.NewBuffer(patchBody))
	reqPatch.Header.Set("Content-Type", "application/json")
	reqPatch.Header.Set("Origin", "http://example.com")
	attachAuthCookies(reqPatch)
	wPatch := httptest.NewRecorder()

	router.ServeHTTP(wPatch, reqPatch)
	if wPatch.Code != http.StatusOK {
		t.Fatalf("E2E Step 4 Assign PIC failed: expected 200 OK, got %d. Body: %s", wPatch.Code, wPatch.Body.String())
	}

	var patchResp struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wPatch.Body.Bytes(), &patchResp)
	currentVersion = patchResp.Data.Version

	// Step 5: Upload Image Attachment
	pngBytes := createTestPNGBytes(10, 10)
	imgBody := &bytes.Buffer{}
	writer := multipart.NewWriter(imgBody)
	part, _ := writer.CreateFormFile("image", "bug_proof.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	reqImg := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments", wsID, itemID), imgBody)
	reqImg.Header.Set("Content-Type", writer.FormDataContentType())
	reqImg.Header.Set("Origin", "http://example.com")
	attachAuthCookies(reqImg)
	wImg := httptest.NewRecorder()

	router.ServeHTTP(wImg, reqImg)
	if wImg.Code != http.StatusCreated {
		t.Fatalf("E2E Step 5 Upload Image failed: expected 201 Created, got %d. Body: %s", wImg.Code, wImg.Body.String())
	}

	// Step 6: Mark Item as DONE
	donePayload := map[string]interface{}{
		"version": currentVersion,
		"status":  "DONE",
	}
	doneBody, _ := json.Marshal(donePayload)
	reqDone := httptest.NewRequest("PATCH", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID, itemID), bytes.NewBuffer(doneBody))
	reqDone.Header.Set("Content-Type", "application/json")
	reqDone.Header.Set("Origin", "http://example.com")
	attachAuthCookies(reqDone)
	wDone := httptest.NewRecorder()

	router.ServeHTTP(wDone, reqDone)
	if wDone.Code != http.StatusOK {
		t.Fatalf("E2E Step 6 Mark DONE failed: expected 200 OK, got %d. Body: %s", wDone.Code, wDone.Body.String())
	}

	// Step 7: Verify Persisted State via GET /items/{id}
	reqGet := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID, itemID), nil)
	attachAuthCookies(reqGet)
	wGet := httptest.NewRecorder()

	router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("E2E Step 7 GET Item failed: expected 200 OK, got %d", wGet.Code)
	}

	var finalResp struct {
		Data struct {
			Status      string     `json:"status"`
			AssigneeID  *uuid.UUID `json:"assignee_id"`
			Type        string     `json:"type"`
			Title       string     `json:"title"`
			Attachments []struct {
				OriginalFilename string `json:"original_filename"`
			} `json:"attachments"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wGet.Body.Bytes(), &finalResp)

	d := finalResp.Data
	if d.Status != "DONE" {
		t.Errorf("E2E Verification failed: expected status DONE, got %s", d.Status)
	}
	if d.AssigneeID == nil || *d.AssigneeID != picID {
		t.Errorf("E2E Verification failed: expected assignee %s, got %v", picID, d.AssigneeID)
	}
	if len(d.Attachments) != 1 || d.Attachments[0].OriginalFilename != "bug_proof.png" {
		t.Errorf("E2E Verification failed: expected 1 attachment 'bug_proof.png', got %v", d.Attachments)
	}
}
