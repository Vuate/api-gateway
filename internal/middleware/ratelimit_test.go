package middleware

import (
	"net/http"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func rlOKHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func newFallbackRL(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		windowMs:         window.Milliseconds(),
		limit:            limit,
		name:             "test",
		fallbackLimiters: make(map[string]*rate.Limiter),
	}
}

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := newFallbackRL(5, time.Second)

	for i := 0; i < 5; i++ {
		if !rl.fallbackAllow("1.1.1.1") {
			t.Errorf("istek %d geçmeli ama engellendi", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := newFallbackRL(3, time.Second)

	for i := 0; i < 3; i++ {
		rl.fallbackAllow("10.0.0.1")
	}

	if rl.fallbackAllow("10.0.0.1") {
		t.Error("limit aşıldı, 4. istek engellenmeli")
	}
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := newFallbackRL(1, time.Second)

	rl.fallbackAllow("1.1.1.1") // IP 1 limiti doldu

	if !rl.fallbackAllow("2.2.2.2") { // IP 2 bağımsız, geçmeli
		t.Error("farklı IP engellenmemeli")
	}
}
