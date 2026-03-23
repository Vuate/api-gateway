package handler

import (
    "net/http"
    "net/http/httputil"
    "net/url"
)

func NewProxy(target string) http.Handler {
    url, _ := url.Parse(target)
    return httputil.NewSingleHostReverseProxy(url)
}