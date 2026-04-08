package middleware

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type state int

const (
	stateClosed   state = iota // normal, istekler geçiyor
	stateOpen                  // servis çöktü, istekleri kes
	stateHalfOpen              // test aşaması, servis düzeldi mi?
)

type CircuitBreaker struct {
	mu              sync.Mutex
	state           state
	failures        int
	maxFailures     int
	timeout         time.Duration
	lastFailureTime time.Time
	serviceName     string
}

func NewCircuitBreaker(serviceName string) *CircuitBreaker {
	return &CircuitBreaker{
		state:       stateClosed,
		maxFailures: 5,
		timeout:     30 * time.Second,
		serviceName: serviceName,
	}
}

func (cb *CircuitBreaker) isOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == stateOpen {
		// 30 saniye geçtiyse half-open'a al, servisi test et
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = stateHalfOpen
			return false
		}
		return true
	}
	return false
}

func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = stateClosed
	SetCircuitBreakerOpen(cb.serviceName, false)
}

func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailureTime = time.Now()
	if cb.failures >= cb.maxFailures {
		cb.state = stateOpen
		SetCircuitBreakerOpen(cb.serviceName, true)
	}
}

func (cb *CircuitBreaker) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cb.isOpen() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "service temporarily unavailable",
				"service": cb.serviceName,
			})
			return
		}

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		if rw.statusCode >= 500 {
			cb.onFailure()
		} else {
			cb.onSuccess()
		}
	})
}

// statusCode'u yakalamak için ResponseWriter wrapper
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("ResponseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}
