package middleware

import (
	"log"
	"net/http"
	"time"
)
// docker compose logs -f api-gateway
// RequestLogger, gateway'e gelen her HTTP isteğini yakalar ve
// method, path, status code, süre, IP ve kullanıcı bilgilerini loglar.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			userID = "-"
		}

		requestID, _ := r.Context().Value(RequestIDKey).(string)
		if requestID == "" {
			requestID = "-"
		}

		log.Printf("[%s] %s %s | status=%d | latency=%dms | ip=%s | user=%s | request_id=%s",
			time.Now().Format("2006-01-02 15:04:05"),
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			time.Since(start).Milliseconds(),
			r.RemoteAddr,
			userID,
			requestID,
		)
	})
}
