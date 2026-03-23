package handler

import (
    "net/http"
    "net/http/httputil"
    "net/url"
)

func NewProxy(target string) http.Handler {
    url, _ := url.Parse(target)
    proxy := httputil.NewSingleHostReverseProxy(url)
    proxy.Director = func(req *http.Request) {
        req.URL.Scheme = url.Scheme
        req.URL.Host = url.Host
        req.Host = url.Host
        req.Header.Set("ngrok-skip-browser-warning", "true")
    }
    return proxy
}