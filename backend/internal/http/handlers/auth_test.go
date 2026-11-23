package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
	"task-bug-tracker/backend/internal/http/middleware"
)

func TestAuthEndpointsIntegration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping auth endpoints integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	sessionStore := auth.NewSessionStore(pool)
	csrfSecret := []byte("super-secret-csrf-key-32-bytes!")
	handler := NewAuthHandler(pool, sessionStore, csrfSecret, false)
	authMiddleware := middleware.RequireAuth(sessionStore, pool, false)

	testUserID := uuid.New()
	testEmail := "authtest_" + testUserID.String()[:8] + "@example.com"
	rawPassword := "InitialPass123!"
	hashedPassword, _ := auth.HashPassword(rawPassword)

	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		testUserID, testEmail, "Auth Test User", hashedPassword, "USER", true, false,
	)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	// 1. Test Login - Success
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": rawPassword,
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Login(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed, expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "sid" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected session cookie 'sid' to be set")
	}

	// 2. Test Login - Failure (Wrong Password)
	badPayload, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": "WrongPassword123!",
	})
	reqBad := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(badPayload))
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()

	handler.Login(wBad, reqBad)
	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for wrong password, got %d", wBad.Code)
	}

	// 3. Test GET /auth/me - Authorized
	reqMe := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	reqMe.AddCookie(sessionCookie)
	wMe := httptest.NewRecorder()

	protectedMeHandler := authMiddleware(http.HandlerFunc(handler.Me))
	protectedMeHandler.ServeHTTP(wMe, reqMe)

	if wMe.Code != http.StatusOK {
		t.Fatalf("expected /auth/me 200, got %d. Body: %s", wMe.Code, wMe.Body.String())
	}

	// 4. Test GET /auth/me - Unauthorized (No Cookie)
	reqMeUnauth := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	wMeUnauth := httptest.NewRecorder()
	protectedMeHandler.ServeHTTP(wMeUnauth, reqMeUnauth)
	if wMeUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing session cookie, got %d", wMeUnauth.Code)
	}

	// 5. Test POST /auth/logout
	reqLogout := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	reqLogout.AddCookie(sessionCookie)
	wLogout := httptest.NewRecorder()

	protectedLogoutHandler := authMiddleware(http.HandlerFunc(handler.Logout))
	protectedLogoutHandler.ServeHTTP(wLogout, reqLogout)

	if wLogout.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for logout, got %d", wLogout.Code)
	}

	// 6. Verify Old Session Reuse Fails After Logout
	wMePostLogout := httptest.NewRecorder()
	protectedMeHandler.ServeHTTP(wMePostLogout, reqMe)
	if wMePostLogout.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when reusing revoked session cookie, got %d", wMePostLogout.Code)
	}
}
