package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/Vuate/api-gateway/config"
	"github.com/Vuate/api-gateway/internal/handler"
	apimiddleware "github.com/Vuate/api-gateway/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := config.Load()

	rps := 10.0
	burst := 30
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			rps = parsed
		}
	}
	if v := os.Getenv("RATE_LIMIT_BURST"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			burst = parsed
		}
	}
	rateLimiter := apimiddleware.NewRateLimiter(rps, burst)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(rateLimiter.Middleware)
	r.Use(apimiddleware.Metrics)

	r.Get("/health", handler.Health(cfg))
	r.Handle("/metrics", promhttp.Handler())

	// Her downstream servis icin bagimsiz circuit breaker
	marketDataCB := apimiddleware.NewCircuitBreaker("market-data")
	exchangeCB := apimiddleware.NewCircuitBreaker("exchange")
	authCB := apimiddleware.NewCircuitBreaker("auth")

	// Auth — JWT gerekmez
	r.Handle("/api/v1/auth/login", authCB.Wrap(handler.NewProxy(cfg.AuthURL)))
	r.Handle("/api/v1/auth/register", authCB.Wrap(handler.NewProxy(cfg.AuthURL)))

	// Public — JWT gerekmez
	r.Handle("/api/v1/quotes/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	r.Handle("/api/v1/ohlcv/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	r.Handle("/api/v1/funding-rate/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	
	r.Handle("/api/v1/compare/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	r.Handle("/api/v1/funding/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	r.Handle("/api/v1/spread/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))

	// WebSocket — circuit breaker gecerli degil, dogrudan proxy
	r.Handle("/ws/quotes/*", handler.NewWebSocketProxy(cfg.MarketDataURL))


	// Protected — JWT zorunlu
	r.Group(func(r chi.Router) {
		r.Use(apimiddleware.JWTAuth(cfg.JWTSecret))
		r.Handle("/positions/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/pnl/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/orders/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/trades/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/dca/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/risk/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/apikeys/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
        r.Handle("/api/v1/apikeys", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))

		// WebSocket — JWT dogrulandiktan sonra proxy'e iletilir
		r.Handle("/ws/positions/*", handler.NewWebSocketProxy(cfg.ExchangeURL))
	})

	log.Printf("Server starting on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
