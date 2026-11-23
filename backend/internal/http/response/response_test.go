package response

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorContract(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), RequestIDKey, "req-12345")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	Error(rec, req, http.StatusBadRequest, "BAD_REQUEST", "Invalid input parameters")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var res ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if res.Error.Code != "BAD_REQUEST" {
		t.Errorf("expected code BAD_REQUEST, got %s", res.Error.Code)
	}
	if res.Error.Message != "Invalid input parameters" {
		t.Errorf("expected message 'Invalid input parameters', got %s", res.Error.Message)
	}
	if res.Error.RequestID != "req-12345" {
		t.Errorf("expected request_id req-12345, got %s", res.Error.RequestID)
	}
}
