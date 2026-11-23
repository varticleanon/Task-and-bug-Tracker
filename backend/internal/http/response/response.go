package response

import (
	"context"
	"encoding/json"
	"net/http"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

type ErrorPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type ErrorResponse struct {
	Error ErrorPayload `json:"error"`
}

func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(RequestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	if v := ctx.Value("request_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func SendJSON(w http.ResponseWriter, status int, data interface{}) {
	// Jaga agar body selalu berisi JSON valid. Nilai nil akan diserialisasi
	// menjadi "null" oleh encoder, yang membuat klien gagal saat membaca
	// res.data. Diganti menjadi object kosong supaya kontrak API stabil.
	if data == nil {
		data = map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func JSON(w http.ResponseWriter, status int, data interface{}) {
	SendJSON(w, status, data)
}

func SendError(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	var reqID string
	if r != nil {
		reqID = GetRequestID(r.Context())
	}

	resp := ErrorResponse{
		Error: ErrorPayload{
			Code:      code,
			Message:   message,
			RequestID: reqID,
		},
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func Error(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	SendError(w, r, status, code, message)
}
