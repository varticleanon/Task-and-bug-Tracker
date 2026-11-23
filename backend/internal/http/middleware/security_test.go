package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"task-bug-tracker/backend/internal/auth"
)

func TestOriginValidation(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/workspaces", nil)
	req.Host = "localhost:8080"

	if ValidateOrigin(req) {
		t.Fatal("expected ValidateOrigin to fail when Origin and Referer are missing")
	}

	req.Header.Set("Origin", "http://evil.com")
	if ValidateOrigin(req) {
		t.Fatal("expected ValidateOrigin to fail for mismatched Origin")
	}

	req.Header.Set("Origin", "http://localhost:8080")
	if !ValidateOrigin(req) {
		t.Fatal("expected ValidateOrigin to succeed for matching Origin")
	}
}

func TestRequireCSRFMiddleware(t *testing.T) {
	secret := []byte("secret-key-32-bytes-long!!")
	csrfMiddleware := RequireCSRF(secret, false)

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	sessionID := uuid.New()
	validToken := GenerateCSRFToken(sessionID.String(), secret)

	reqNoCSRF := httptest.NewRequest("POST", "/api/v1/workspaces", nil)
	reqNoCSRF.Host = "localhost:8080"
	reqNoCSRF.Header.Set("Origin", "http://localhost:8080")

	session := &auth.Session{ID: sessionID}
	ctx := context.WithValue(reqNoCSRF.Context(), SessionContextKey, session)
	reqNoCSRF = reqNoCSRF.WithContext(ctx)

	w1 := httptest.NewRecorder()
	csrfMiddleware(dummyHandler).ServeHTTP(w1, reqNoCSRF)
	if w1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for missing CSRF token, got %d", w1.Code)
	}

	reqValid := httptest.NewRequest("POST", "/api/v1/workspaces", nil)
	reqValid.Host = "localhost:8080"
	reqValid.Header.Set("Origin", "http://localhost:8080")
	reqValid.Header.Set("X-XSRF-TOKEN", validToken)
	reqValid = reqValid.WithContext(ctx)

	w2 := httptest.NewRecorder()
	csrfMiddleware(dummyHandler).ServeHTTP(w2, reqValid)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid CSRF token, got %d", w2.Code)
	}
}

func TestLoginRateLimiter(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 1*time.Minute)
	middleware := limiter.LimitMiddleware()

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()
		middleware(dummyHandler).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected attempt %d to pass (200 OK), got %d", i, w.Code)
		}
	}

	reqExcess := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	reqExcess.RemoteAddr = "192.168.1.100:12345"
	wExcess := httptest.NewRecorder()
	middleware(dummyHandler).ServeHTTP(wExcess, reqExcess)

	if wExcess.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429 Too Many Requests, got %d", wExcess.Code)
	}
}
