package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/Vuate/api-gateway/internal/middleware"
)

func NewProxy(target string) http.Handler {
	u, _ := url.Parse(target)
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
	return proxy
}