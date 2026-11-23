package middleware

import (
	"net/http"
)

func Cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Izinkan origin dari code-server domain dan localhost development
		allowedOrigins := []string{
			"http://localhost:4200",
			"https://code-server.skyxserver.online",
		}

		// Cek apakah origin ada dalam daftar yang diizinkan
		allowed := false
		for _, ao := range allowedOrigins {
			if origin == ao {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-XSRF-TOKEN, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
