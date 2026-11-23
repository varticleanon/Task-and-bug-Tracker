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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/storage"
)

func TestPhase3Gate_SecurityAndUXAcceptance(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping Phase 3 Gate test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	tmpDir, err := os.MkdirTemp("", "p3_gate_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	router := setupE2ERouter(pool, sessionStore, store)

	// 1. Setup Active User & Workspace
	userID := uuid.New()
	userEmail := "p3gate_user_" + userID.String()[:8] + "@example.com"
	rawPassword := "Password123!"
	passHash, _ := auth.HashPassword(rawPassword)
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'P3 Gate User', $3, 'USER', true, false)", userID, userEmail, passHash)

	wsID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'P3 Gate Workspace', $2, 'WS P3', $3)", wsID, "p3gate-"+wsID.String()[:8], userID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsID, userID)

	// 2. Login via /auth/login
	loginBody, _ := json.Marshal(map[string]string{"email": userEmail, "password": rawPassword})
	reqLogin := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	reqLogin.Header.Set("Origin", "http://example.com")
	wLogin := httptest.NewRecorder()
	router.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusOK {
		t.Fatalf("Gate P3 Login failed with status %d", wLogin.Code)
	}
	cookies := wLogin.Result().Cookies()

	attachCookies := func(req *http.Request) {
		for _, c := range cookies {
			req.AddCookie(c)
		}
	}

	// 3. Create BUG item
	bugPayload, _ := json.Marshal(map[string]interface{}{
		"type":        "BUG",
		"title":       "P3 Gate Upload & UX Bug",
		"description": "Verifying Phase 3 Security & UX Requirements",
		"status":      "TODO",
		"priority":    "CRITICAL",
	})
	reqBug := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items", wsID), bytes.NewBuffer(bugPayload))
	reqBug.Header.Set("Content-Type", "application/json")
	reqBug.Header.Set("Origin", "http://example.com")
	attachCookies(reqBug)
	wBug := httptest.NewRecorder()
	router.ServeHTTP(wBug, reqBug)

	if wBug.Code != http.StatusCreated {
		t.Fatalf("Gate P3 Item creation failed with status %d", wBug.Code)
	}

	var itemResp struct {
		Data struct {
			ID      uuid.UUID `json:"id"`
			Version int       `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wBug.Body.Bytes(), &itemResp)
	itemID := itemResp.Data.ID

	// 4. Upload Valid Image Attachment (IMG-01)
	pngBytes := createTestPNGBytes(10, 10)
	imgBody := &bytes.Buffer{}
	writer := multipart.NewWriter(imgBody)
	part, _ := writer.CreateFormFile("image", "gate_proof.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	reqImg := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments", wsID, itemID), imgBody)
	reqImg.Header.Set("Content-Type", writer.FormDataContentType())
	reqImg.Header.Set("Origin", "http://example.com")
	attachCookies(reqImg)
	wImg := httptest.NewRecorder()
	router.ServeHTTP(wImg, reqImg)

	if wImg.Code != http.StatusCreated {
		t.Fatalf("Gate P3 Attachment upload failed with status %d", wImg.Code)
	}

	// 5. Verify persistent state (UX-01)
	reqGet := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s", wsID, itemID), nil)
	attachCookies(reqGet)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("Gate P3 GET Item failed with status %d", wGet.Code)
	}

	var getResp struct {
		Data struct {
			Title       string `json:"title"`
			Attachments []struct {
				OriginalFilename string `json:"original_filename"`
			} `json:"attachments"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wGet.Body.Bytes(), &getResp)

	if len(getResp.Data.Attachments) != 1 || getResp.Data.Attachments[0].OriginalFilename != "gate_proof.png" {
		t.Errorf("Gate P3 attachment verification failed: %v", getResp.Data.Attachments)
	}
}
