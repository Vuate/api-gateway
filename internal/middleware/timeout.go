package middleware

import (
	"context"
	"net/http"
	"time"
)

// TimeoutMiddleware, isteği verilen süre sonunda iptal eder.
// WebSocket route'larında timeout olmaması için 0 geçilir.
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if timeout == 0 {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
