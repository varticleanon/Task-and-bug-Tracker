package middleware

import (
	"net/http"
	"strings"
)

/**
 * StripProxyPrefix — menghapus prefix /absproxy/* dari URL path
 * sebelum route matching di backend.
 *
 * Ketika request datang melalui code-server reverse proxy:
 *   Browser -> https://code-server.skyxserver.online/absproxy/8081/api/v1/auth/login
 *   code-server -> http://localhost:8081/absproxy/8081/api/v1/auth/login
 *
 * Backend hanya mengenal route /api/v1/auth/login, jadi prefix /absproxy/8081
 * harus dihapus agar route matching berfungsi.
 */
func StripProxyPrefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Strip /absproxy/8081 prefix (backend port via code-server)
		if strings.HasPrefix(path, "/absproxy/8081") {
			r.URL.Path = strings.TrimPrefix(path, "/absproxy/8081")
		}

		// Strip /absproxy/4200 prefix (frontend port via code-server, untuk fallback)
		if strings.HasPrefix(path, "/absproxy/4200") {
			r.URL.Path = strings.TrimPrefix(path, "/absproxy/4200")
		}

		next.ServeHTTP(w, r)
	})
}
