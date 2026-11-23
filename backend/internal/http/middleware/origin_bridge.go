package middleware

import (
	"net/http"
)

func OriginValidation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validasi Origin hanya berlaku untuk request mutasi (non-GET/HEAD/OPTIONS)
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if !ValidateOrigin(r) {
				http.Error(w, "Forbidden: Invalid Origin", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
