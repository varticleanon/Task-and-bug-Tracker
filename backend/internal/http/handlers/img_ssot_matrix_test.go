package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
	"task-bug-tracker/backend/internal/storage"
)

func createTestPNGBytes(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func setupImageTestRouter(pool *pgxpool.Pool, sessionStore *auth.SessionStore, store storage.Storage) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.OriginValidation)

	authHandler := NewAuthHandler(pool, sessionStore, []byte("super-secret-key-for-dev-only"), true)

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(sessionStore, pool, false))
		r.Get("/api/v1/auth/me", authHandler.Me)

		r.Route("/api/v1/workspaces/{workspaceId}", func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRole(pool, "VIEWER"))

			attachHandler := NewAttachmentHandler(pool, store)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Post("/items/{id}/attachments", attachHandler.UploadAttachment)
			r.Get("/items/{id}/attachments/{aid}/content", attachHandler.GetAttachmentContent)
			r.With(middleware.RequireWorkspaceRole(pool, "MEMBER")).Delete("/items/{id}/attachments/{aid}", attachHandler.DeleteAttachment)
		})
	})
	return r
}

func TestIMG01_UploadValidImageAndFetchContent(t *testing.T) {
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
	tmpDir, _ := os.MkdirTemp("", "img_test_*")
	defer os.RemoveAll(tmpDir)
	store, _ := storage.NewLocalStorage(tmpDir)

	router := setupImageTestRouter(pool, sessionStore, store)

	userAID := uuid.New()
	passHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Img User A', $3, 'USER', true, false)", userAID, "img_a_"+userAID.String()[:8]+"@example.com", passHash)
	tokenUserA, _, _ := sessionStore.CreateSession(ctx, userAID, "127.0.0.1", "TestAgent")

	wsAID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS Img A', $2, 'WS Img', $3)", wsAID, "wsimg-a-"+wsAID.String()[:8], userAID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'MEMBER')", wsAID, userAID)

	itemID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Img Item', 'TODO', 'MEDIUM', $3, $3)", itemID, wsAID, userAID)

	// 1. Upload
	pngBytes := createTestPNGBytes(10, 10)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("image", "sample.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	uploadReq := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments", wsAID, itemID), body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadReq.Header.Set("Origin", "http://example.com")
	uploadReq.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	wUpload := httptest.NewRecorder()

	router.ServeHTTP(wUpload, uploadReq)
	if wUpload.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for upload, got %d. Body: %s", wUpload.Code, wUpload.Body.String())
	}

	var uploadResp struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wUpload.Body.Bytes(), &uploadResp)
	attachID := uploadResp.Data.ID

	// 2. Fetch Content
	fetchReq := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments/%s/content", wsAID, itemID, attachID), nil)
	fetchReq.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	wFetch := httptest.NewRecorder()

	router.ServeHTTP(wFetch, fetchReq)
	if wFetch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for content, got %d", wFetch.Code)
	}

	if wFetch.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff")
	}
	if wFetch.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("expected Cache-Control: private, no-store")
	}
}

func TestIMG02_RejectInvalidImage(t *testing.T) {
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
	tmpDir, _ := os.MkdirTemp("", "img_test_*")
	defer os.RemoveAll(tmpDir)
	store, _ := storage.NewLocalStorage(tmpDir)

	router := setupImageTestRouter(pool, sessionStore, store)

	userAID := uuid.New()
	passHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Img User', $3, 'USER', true, false)", userAID, "img_b_"+userAID.String()[:8]+"@example.com", passHash)
	tokenUserA, _, _ := sessionStore.CreateSession(ctx, userAID, "127.0.0.1", "TestAgent")

	wsAID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS Img B', $2, 'WS Img', $3)", wsAID, "wsimg-b-"+wsAID.String()[:8], userAID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'MEMBER')", wsAID, userAID)

	itemID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Img Item B', 'TODO', 'MEDIUM', $3, $3)", itemID, wsAID, userAID)

	// Fake image with invalid bytes
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("image", "malicious.png")
	_, _ = part.Write([]byte("NOT_A_REAL_IMAGE_FILE_BYTES"))
	_ = writer.Close()

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments", wsAID, itemID), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType && w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 415 or 422 for fake image, got %d", w.Code)
	}
}

func TestIMG03_MaxFiveImagesLimitEnforced(t *testing.T) {
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
	tmpDir, _ := os.MkdirTemp("", "img_test_*")
	defer os.RemoveAll(tmpDir)
	store, _ := storage.NewLocalStorage(tmpDir)

	router := setupImageTestRouter(pool, sessionStore, store)

	userAID := uuid.New()
	passHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'Img User C', $3, 'USER', true, false)", userAID, "img_c_"+userAID.String()[:8]+"@example.com", passHash)
	tokenUserA, _, _ := sessionStore.CreateSession(ctx, userAID, "127.0.0.1", "TestAgent")

	wsAID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS Img C', $2, 'WS Img', $3)", wsAID, "wsimg-c-"+wsAID.String()[:8], userAID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'MEMBER')", wsAID, userAID)

	itemID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Img Item C', 'TODO', 'MEDIUM', $3, $3)", itemID, wsAID, userAID)

	pngBytes := createTestPNGBytes(5, 5)

	// Upload 5 valid images sequentially
	for i := 1; i <= 5; i++ {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("image", fmt.Sprintf("img_%d.png", i))
		_, _ = part.Write(pngBytes)
		_ = writer.Close()

		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments", wsAID, itemID), body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Origin", "http://example.com")
		req.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected image %d to be created (201), got %d. Body: %s", i, w.Code, w.Body.String())
		}
	}

	// Attempt 6th upload -> MUST fail with 400 Bad Request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("image", "img_6.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req6 := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments", wsAID, itemID), body)
	req6.Header.Set("Content-Type", writer.FormDataContentType())
	req6.Header.Set("Origin", "http://example.com")
	req6.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	w6 := httptest.NewRecorder()

	router.ServeHTTP(w6, req6)
	if w6.Code != http.StatusBadRequest {
		t.Fatalf("expected 6th upload to return 400 Bad Request, got %d", w6.Code)
	}

	// Concurrent race-condition test for 5-image cap
	itemID2 := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Concurrent Item', 'TODO', 'MEDIUM', $3, $3)", itemID2, wsAID, userAID)

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 7; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b := &bytes.Buffer{}
			w := multipart.NewWriter(b)
			p, _ := w.CreateFormFile("image", fmt.Sprintf("conc_%d.png", idx))
			_, _ = p.Write(pngBytes)
			_ = w.Close()

			r := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments", wsAID, itemID2), b)
			r.Header.Set("Content-Type", w.FormDataContentType())
			r.Header.Set("Origin", "http://example.com")
			r.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, r)
			if rec.Code == http.StatusCreated {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if successCount > 5 {
		t.Errorf("concurrent limit overrun! Expected at most 5 successful uploads, got %d", successCount)
	}
}

func TestIMG04_CrossTenantImageFetchProtection(t *testing.T) {
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
	tmpDir, _ := os.MkdirTemp("", "img_test_*")
	defer os.RemoveAll(tmpDir)
	store, _ := storage.NewLocalStorage(tmpDir)

	router := setupImageTestRouter(pool, sessionStore, store)

	// User A in Workspace A
	userAID := uuid.New()
	passHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'User A', $3, 'USER', true, false)", userAID, "img_ct_a_"+userAID.String()[:8]+"@example.com", passHash)
	wsAID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS A', $2, 'WS A', $3)", wsAID, "ws-a-"+wsAID.String()[:8], userAID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsAID, userAID)

	itemID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Item A', 'TODO', 'MEDIUM', $3, $3)", itemID, wsAID, userAID)

	// User B in Workspace B (no access to A)
	userBID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'User B', $3, 'USER', true, false)", userBID, "img_ct_b_"+userBID.String()[:8]+"@example.com", passHash)
	tokenUserB, _, _ := sessionStore.CreateSession(ctx, userBID, "127.0.0.1", "TestAgent")
	wsBID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS B', $2, 'WS B', $3)", wsBID, "ws-b-"+wsBID.String()[:8], userBID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsBID, userBID)

	// User A uploads an image to Item A
	pngBytes := createTestPNGBytes(10, 10)
	storageKey := uuid.New().String() + ".png"
	_ = store.Save(storageKey, pngBytes)

	attachID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO item_attachments (id, workspace_id, item_id, uploader_id, original_filename, storage_key, mime_type, byte_size, width, height) VALUES ($1, $2, $3, $4, 'test.png', $5, 'image/png', 100, 10, 10)", attachID, wsAID, itemID, userAID, storageKey)

	// User B tries to fetch image from Workspace A -> MUST return 404 Not Found
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments/%s/content", wsAID, itemID, attachID), nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserB})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found on cross-tenant image fetch, got %d", w.Code)
	}
}

func TestIMG05_DeleteAttachment(t *testing.T) {
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
	tmpDir, _ := os.MkdirTemp("", "img_test_*")
	defer os.RemoveAll(tmpDir)
	store, _ := storage.NewLocalStorage(tmpDir)

	router := setupImageTestRouter(pool, sessionStore, store)

	userAID := uuid.New()
	passHash, _ := auth.HashPassword("Password123!")
	_, _ = pool.Exec(ctx, "INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, 'User Del', $3, 'USER', true, false)", userAID, "img_del_"+userAID.String()[:8]+"@example.com", passHash)
	tokenUserA, _, _ := sessionStore.CreateSession(ctx, userAID, "127.0.0.1", "TestAgent")

	wsAID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO workspaces (id, name, slug, description, created_by) VALUES ($1, 'WS Del', $2, 'WS Del', $3)", wsAID, "ws-del-"+wsAID.String()[:8], userAID)
	_, _ = pool.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'ADMIN')", wsAID, userAID)

	itemID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, status, priority, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Item Del', 'TODO', 'MEDIUM', $3, $3)", itemID, wsAID, userAID)

	pngBytes := createTestPNGBytes(10, 10)
	storageKey := uuid.New().String() + ".png"
	_ = store.Save(storageKey, pngBytes)

	attachID := uuid.New()
	_, _ = pool.Exec(ctx, "INSERT INTO item_attachments (id, workspace_id, item_id, uploader_id, original_filename, storage_key, mime_type, byte_size, width, height) VALUES ($1, $2, $3, $4, 'test.png', $5, 'image/png', 100, 10, 10)", attachID, wsAID, itemID, userAID, storageKey)

	// Delete attachment
	delReq := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s/items/%s/attachments/%s", wsAID, itemID, attachID), nil)
	delReq.Header.Set("Origin", "http://example.com")
	delReq.AddCookie(&http.Cookie{Name: "sid", Value: tokenUserA})
	wDel := httptest.NewRecorder()

	router.ServeHTTP(wDel, delReq)
	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on delete attachment, got %d", wDel.Code)
	}

	// Verify file is deleted from local storage
	fileLoc := filepath.Join(tmpDir, storageKey)
	if _, err := os.Stat(fileLoc); !os.IsNotExist(err) {
		t.Errorf("expected physical file to be removed from disk, but it still exists")
	}
}
