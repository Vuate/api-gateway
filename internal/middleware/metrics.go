package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	rateLimitHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rate_limit_hits_total",
		Help: "Total number of requests rejected by rate limiter",
	})

	circuitBreakerOpen = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_open",
		Help: "1 if circuit breaker is open, 0 otherwise",
	}, []string{"service"})
)

// RecordRateLimitHit rate limit sayacını bir artırır.
func RecordRateLimitHit() {
	rateLimitHitsTotal.Inc()
}

// SetCircuitBreakerOpen bir servis icin circuit breaker gauge degerini gunceller.
func SetCircuitBreakerOpen(service string, open bool) {
	val := 0.0
	if open {
		val = 1.0
	}
	circuitBreakerOpen.WithLabelValues(service).Set(val)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("ResponseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

// Metrics her istek icin Prometheus metriclerini kaydeden middleware'i dondurur.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(rec, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rec.status)
		path := r.URL.Path

		httpRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}
