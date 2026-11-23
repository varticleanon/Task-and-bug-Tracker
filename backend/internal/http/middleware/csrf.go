package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"task-bug-tracker/backend/internal/http/response"
)

func GenerateCSRFToken(sessionID string, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(sessionID))
	return hex.EncodeToString(h.Sum(nil))
}

func ValidateCSRFToken(sessionID string, token string, secret []byte) bool {
	if token == "" || sessionID == "" {
		return false
	}
	expected := GenerateCSRFToken(sessionID, secret)
	return hmac.Equal([]byte(expected), []byte(token))
}

func ValidateOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return false
	}

	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}

	// Izinkan origin yang cocok dengan Host header (proxy mode)
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}

	// Izinkan origin dari frontend development (localhost:4200) saat backend di port berbeda
	if strings.EqualFold(u.Host, "localhost:4200") || strings.EqualFold(u.Host, "127.0.0.1:4200") {
		return true
	}

	return false
}

func RequireCSRF(secret []byte, isProd bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isMutating := r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" || r.Method == "DELETE"
			if !isMutating {
				next.ServeHTTP(w, r)
				return
			}

			if !ValidateOrigin(r) {
				response.SendError(w, r, http.StatusForbidden, "INVALID_ORIGIN", "Cross-origin requests are forbidden")
				return
			}

			if r.URL.Path == "/api/v1/auth/login" || strings.HasSuffix(r.URL.Path, "/auth/login") {
				if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					response.SendError(w, r, http.StatusBadRequest, "INVALID_CONTENT_TYPE", "Content-Type must be application/json")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			session, hasSession := GetSessionFromContext(r.Context())
			if hasSession && session != nil {
				csrfHeader := r.Header.Get("X-XSRF-TOKEN")
				if csrfHeader == "" || !ValidateCSRFToken(session.ID.String(), csrfHeader, secret) {
					response.SendError(w, r, http.StatusForbidden, "CSRF_FAILED", "Invalid or missing CSRF token")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
