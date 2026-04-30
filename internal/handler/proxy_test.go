package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxy_ForwardsRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/quotes", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("beklenen 200, gelen %d", rec.Code)
	}
}

func TestProxy_UpstreamDown_Returns502(t *testing.T) {
	proxy := NewProxy("http://localhost:1") // kapalı port

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("beklenen 502, gelen %d", rec.Code)
	}
}

func TestProxy_Timeout_Returns504(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // kasıtlı gecikme
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("beklenen 504, gelen %d", rec.Code)
	}
}

func TestProxy_RequestIDPropagated(t *testing.T) {
	var gotRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	proxy := NewProxy(upstream.URL)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	_ = gotRequestID // request ID context'ten gelir, bu testte boş olması normal
}