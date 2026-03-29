package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 500 döndüren sahte servis
func failingHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
}

// 200 döndüren sahte servis
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	cb := NewCircuitBreaker("test-service")

	handler := cb.Wrap(failingHandler())

	// 5 hata gönder — devre açılmalı
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		if rw.Code != http.StatusInternalServerError {
			t.Fatalf("istek %d: 500 beklendi, %d geldi", i+1, rw.Code)
		}
	}

	// 6. istek — devre açık olmalı, 503 gelmeli
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusServiceUnavailable {
		t.Fatalf("devre açık olmalıydı: 503 beklendi, %d geldi", rw.Code)
	}

	t.Log("✓ Circuit breaker 5 hatadan sonra açıldı")
}

func TestCircuitBreaker_ClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker("test-service")

	handler := cb.Wrap(okHandler())

	// 10 başarılı istek — devre kapalı kalmalı
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		if rw.Code != http.StatusOK {
			t.Fatalf("istek %d: 200 beklendi, %d geldi", i+1, rw.Code)
		}
	}

	t.Log("✓ Başarılı isteklerde devre kapalı kaldı")
}
