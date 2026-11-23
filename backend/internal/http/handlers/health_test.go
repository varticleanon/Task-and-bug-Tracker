package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthLive(t *testing.T) {
	handler := NewHealthHandler(nil)
	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	handler.Live(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"status":"live"`) {
		t.Fatalf("expected status 'live', got %s", w.Body.String())
	}
}

func TestHealthReadyUninitialized(t *testing.T) {
	handler := NewHealthHandler(nil)
	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()

	handler.Ready(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("expected status 'not_ready', got %s", w.Body.String())
	}
}
