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

func TestAdminUsersAndGuardIntegrations(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping admin user integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	// Pool ditutup lewat t.Cleanup (LIFO) — bukan defer — supaya cleanup
	// pemulihan state yang didaftarkan belakangan tetap punya pool hidup.
	t.Cleanup(func() { pool.Close() })

	sessionStore := auth.NewSessionStore(pool)
	adminHandler := NewAdminUserHandler(pool, sessionStore)
	authMiddleware := middleware.RequireAuth(sessionStore, pool, false)
	superAdminMiddleware := middleware.RequireSuperAdmin

	// 1. Setup Super Admin User & Normal User
	saID := uuid.New()
	saEmail := "sa_test_" + saID.String()[:8] + "@example.com"
	saPassHash, _ := auth.HashPassword("SuperAdminPass123!")
	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, $3, $4, 'SUPER_ADMIN', true, false)",
		saID, saEmail, "Super Admin", saPassHash,
	)
	if err != nil {
		t.Fatalf("failed to insert test super admin: %v", err)
	}

	tokenSA, _, err := sessionStore.CreateSession(ctx, saID, "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// 2. Test Super Admin Creates New User (POST /admin/users)
	createPayload, _ := json.Marshal(map[string]string{
		"email": "newuser_" + uuid.New().String()[:8] + "@example.com",
		"name":  "New Member",
	})
	reqCreate := httptest.NewRequest("POST", "/api/v1/admin/users", bytes.NewBuffer(createPayload))
	reqCreate.AddCookie(&http.Cookie{Name: "sid", Value: tokenSA})
	wCreate := httptest.NewRecorder()

	protectedCreate := authMiddleware(superAdminMiddleware(http.HandlerFunc(adminHandler.CreateUser)))
	protectedCreate.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for user creation, got %d. Body: %s", wCreate.Code, wCreate.Body.String())
	}

	var createResp map[string]map[string]interface{}
	_ = json.Unmarshal(wCreate.Body.Bytes(), &createResp)
	tempPass, ok := createResp["data"]["temporary_password"].(string)
	if !ok || tempPass == "" {
		t.Fatal("expected temporary_password to be returned")
	}

	// 3. Test Forced Password Change Restriction (must_change_password = true)
	forcedUserID := uuid.New()
	forcedEmail := "forced_" + forcedUserID.String()[:8] + "@example.com"
	forcedPassHash, _ := auth.HashPassword(tempPass)
	_, _ = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, $3, $4, 'USER', true, true)",
		forcedUserID, forcedEmail, "Forced User", forcedPassHash,
	)

	tokenForced, _, _ := sessionStore.CreateSession(ctx, forcedUserID, "127.0.0.1", "TestAgent")

	reqBlocked := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	reqBlocked.AddCookie(&http.Cookie{Name: "sid", Value: tokenForced})
	wBlocked := httptest.NewRecorder()

	protectedList := authMiddleware(http.HandlerFunc(adminHandler.ListUsers))
	protectedList.ServeHTTP(wBlocked, reqBlocked)

	if wBlocked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for user with must_change_password=true, got %d", wBlocked.Code)
	}

	// 4. Test Non-Super-Admin Forbidden Guard (HTTP 403)
	normalUserID := uuid.New()
	normalEmail := "normal_" + normalUserID.String()[:8] + "@example.com"
	normalPassHash, _ := auth.HashPassword("UserPassword123!")
	_, _ = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, $3, $4, 'USER', true, false)",
		normalUserID, normalEmail, "Normal User", normalPassHash,
	)
	tokenNormal, _, _ := sessionStore.CreateSession(ctx, normalUserID, "127.0.0.1", "TestAgent")

	reqForbidden := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	reqForbidden.AddCookie(&http.Cookie{Name: "sid", Value: tokenNormal})
	wForbidden := httptest.NewRecorder()

	protectedSuperAdminOnly := authMiddleware(superAdminMiddleware(http.HandlerFunc(adminHandler.ListUsers)))
	protectedSuperAdminOnly.ServeHTTP(wForbidden, reqForbidden)

	if wForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when normal USER accesses admin users endpoint, got %d", wForbidden.Code)
	}

	// 5. Test Last Super Admin Guard (Cannot disable sole active Super Admin)
	//
	// Guard ini menghitung SELURUH Super Admin aktif di DB. Agar test ini
	// meaningful, Super Admin lain (termasuk akun produksi) dinonaktifkan
	// sementara — lalu dikembalikan ke status awal saat test selesai.
	// Jangan pernah meninggalkan akun produksi dalam keadaan disabled.
	otherActiveSAIDs := []uuid.UUID{}
	rowsSA, err := pool.Query(ctx,
		"SELECT id FROM users WHERE global_role = 'SUPER_ADMIN' AND is_active = true AND id != $1",
		saID)
	if err != nil {
		t.Fatalf("failed to list other active super admins: %v", err)
	}
	for rowsSA.Next() {
		var id uuid.UUID
		if err := rowsSA.Scan(&id); err != nil {
			rowsSA.Close()
			t.Fatalf("failed to scan super admin id: %v", err)
		}
		otherActiveSAIDs = append(otherActiveSAIDs, id)
	}
	rowsSA.Close()

	t.Cleanup(func() {
		for _, id := range otherActiveSAIDs {
			if _, err := pool.Exec(ctx, "UPDATE users SET is_active = true WHERE id = $1", id); err != nil {
				t.Errorf("failed to restore super admin %s to active: %v", id, err)
			}
		}
	})

	if len(otherActiveSAIDs) > 0 {
		_, err = pool.Exec(ctx,
			"UPDATE users SET is_active = false WHERE global_role = 'SUPER_ADMIN' AND id != $1",
			saID)
		if err != nil {
			t.Fatalf("failed to isolate sole super admin in test: %v", err)
		}
	}

	r := chi.NewRouter()
	r.Use(authMiddleware, superAdminMiddleware)
	r.Patch("/api/v1/admin/users/{userId}", adminHandler.UpdateUser)

	disablePayload, _ := json.Marshal(map[string]interface{}{"is_active": false})
	reqDisableSA := httptest.NewRequest("PATCH", "/api/v1/admin/users/"+saID.String(), bytes.NewBuffer(disablePayload))
	reqDisableSA.AddCookie(&http.Cookie{Name: "sid", Value: tokenSA})
	wDisableSA := httptest.NewRecorder()

	r.ServeHTTP(wDisableSA, reqDisableSA)

	if wDisableSA.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when attempting to disable sole Super Admin, got %d. Body: %s", wDisableSA.Code, wDisableSA.Body.String())
	}

	// 6. Regression: list user harus selalu mengembalikan JSON valid dengan
	//    "data" berupa array (bukan null) dan "meta" yang lengkap.
	reqList := httptest.NewRequest("GET", "/api/v1/admin/users?page=1&page_size=20", nil)
	reqList.AddCookie(&http.Cookie{Name: "sid", Value: tokenSA})
	wList := httptest.NewRecorder()

	protectedList.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for user list, got %d. Body: %s", wList.Code, wList.Body.String())
	}

	var listResp struct {
		Data []map[string]interface{} `json:"data"`
		Meta *struct {
			Page       int `json:"page"`
			TotalPages int `json:"total_pages"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(wList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("user list response is not valid JSON: %v. Body: %s", err, wList.Body.String())
	}
	if listResp.Data == nil {
		t.Fatalf("expected user list 'data' to be an array (not null). Body: %s", wList.Body.String())
	}
	if listResp.Meta == nil {
		t.Fatalf("expected user list 'meta' to be present. Body: %s", wList.Body.String())
	}
}
