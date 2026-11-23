package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"task-bug-tracker/backend/internal/http/response"
)

type rateLimitEntry struct {
	count     int
	resetTime time.Time
}

type LoginRateLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*rateLimitEntry
	maxAttempts int
	window      time.Duration
}

func NewLoginRateLimiter(maxAttempts int, window time.Duration) *LoginRateLimiter {
	limiter := &LoginRateLimiter{
		attempts:    make(map[string]*rateLimitEntry),
		maxAttempts: maxAttempts,
		window:      window,
	}

	go func() {
		for {
			time.Sleep(window)
			limiter.mu.Lock()
			now := time.Now()
			for key, entry := range limiter.attempts {
				if now.After(entry.resetTime) {
					delete(limiter.attempts, key)
				}
			}
			limiter.mu.Unlock()
		}
	}()

	return limiter
}

func (l *LoginRateLimiter) LimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isAuthMutating := r.Method == "POST" &&
				(strings.HasSuffix(r.URL.Path, "/auth/login") ||
					r.URL.Path == "/api/v1/auth/login" ||
					strings.HasSuffix(r.URL.Path, "/auth/change-password") ||
					strings.HasSuffix(r.URL.Path, "/auth/reset-password"))

			if !isAuthMutating {
				next.ServeHTTP(w, r)
				return
			}

			ip := r.RemoteAddr
			if idx := strings.LastIndex(ip, ":"); idx != -1 {
				ip = ip[:idx]
			}

			key := "ip:" + ip

			l.mu.Lock()
			now := time.Now()
			entry, exists := l.attempts[key]
			if !exists || now.After(entry.resetTime) {
				l.attempts[key] = &rateLimitEntry{
					count:     1,
					resetTime: now.Add(l.window),
				}
				l.mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}

			if entry.count >= l.maxAttempts {
				l.mu.Unlock()
				response.SendError(w, r, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Too many authentication attempts. Please try again later.")
				return
			}

			entry.count++
			l.mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}
