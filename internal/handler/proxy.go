package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/Vuate/api-gateway/internal/middleware"
)

func NewProxy(target string) http.Handler {
	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("[proxy] geçersiz upstream URL %q: %v", target, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = u.Scheme
		req.URL.Host = u.Host
		req.Host = u.Host
		req.Header.Set("ngrok-skip-browser-warning", "true")

		if id, ok := req.Context().Value(middleware.RequestIDKey).(string); ok && id != "" {
			req.Header.Set("X-Request-ID", id)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			http.Error(w, "upstream timeout", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	return proxy
}