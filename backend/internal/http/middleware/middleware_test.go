package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task-bug-tracker/backend/internal/http/response"
)

func TestRequestIDMiddleware(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := response.GetRequestID(r.Context())
		if reqID == "" {
			t.Error("expected request_id in context, got empty")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	respReqID := rec.Header().Get("X-Request-ID")
	if respReqID == "" {
		t.Error("expected X-Request-ID header in response")
	}
}

func TestRecovererMiddleware(t *testing.T) {
	handler := RequestID(Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went terribly wrong")
	})))

	req := httptest.NewRequest("GET", "/panic", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d on panic, got %d", http.StatusInternalServerError, rec.Code)
	}

	var res response.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if res.Error.Code != "INTERNAL_SERVER_ERROR" {
		t.Errorf("expected code INTERNAL_SERVER_ERROR, got %s", res.Error.Code)
	}
}
