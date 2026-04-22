package middleware

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type state int

const (
	stateClosed   state = iota // normal, istekler geçiyor
	stateOpen                  // servis çöktü, istekleri kes
	stateHalfOpen              // test aşaması, servis düzeldi mi?
)

type CircuitBreakerConfig struct {
	FailureThreshold int           // kaç 5xx sonra open
	SuccessThreshold int           // half-open'dan closed için kaç başarı
	Timeout          time.Duration // open → half-open bekleme süresi
}

func loadCBConfig(service string) CircuitBreakerConfig {
	prefix := strings.ReplaceAll(strings.ToUpper(service), "-", "_")
	return CircuitBreakerConfig{
		FailureThreshold: getEnvInt(prefix+"_CB_FAILURE_THRESHOLD", 5),
		SuccessThreshold: getEnvInt(prefix+"_CB_SUCCESS_THRESHOLD", 2),
		Timeout:          getEnvDuration(prefix+"_CB_TIMEOUT", 30*time.Second),
	}
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

type CircuitBreaker struct {
	mu               sync.Mutex
	state            state
	failures         int
	successes        int
	maxFailures      int
	successThreshold int
	timeout          time.Duration
	lastFailureTime  time.Time
	serviceName      string
}

func NewCircuitBreaker(serviceName string) *CircuitBreaker {
	cfg := loadCBConfig(serviceName)
	return &CircuitBreaker{
		state:            stateClosed,
		maxFailures:      cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		timeout:          cfg.Timeout,
		serviceName:      serviceName,
	}
}

func (cb *CircuitBreaker) isOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == stateOpen {
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = stateHalfOpen
			cb.successes = 0
			return false
		}
		return true
	}
	return false
}

func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == stateHalfOpen {
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.failures = 0
			cb.successes = 0
			cb.state = stateClosed
			SetCircuitBreakerOpen(cb.serviceName, false)
		}
	} else {
		cb.failures = 0
		cb.state = stateClosed
		SetCircuitBreakerOpen(cb.serviceName, false)
	}
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
