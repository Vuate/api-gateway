package main

import (
	"log"
	"net/http"

	"github.com/Vuate/api-gateway/config"
	"github.com/Vuate/api-gateway/internal/handler"
	apimiddleware "github.com/Vuate/api-gateway/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.Load()

	rateLimiter := apimiddleware.NewRateLimiter(10, 30) // saniyede 10 istek, 30 burst

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(rateLimiter.Middleware)

	r.Get("/health", handler.Health(cfg))

	marketDataCB := apimiddleware.NewCircuitBreaker("market-data")
	exchangeCB := apimiddleware.NewCircuitBreaker("exchange")

	// Public — JWT gerekmez
	r.Handle("/api/v1/quotes/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	r.Handle("/ws/quotes/*", handler.NewWebSocketProxy(cfg.MarketDataURL))

	// Protected — JWT zorunlu
	r.Group(func(r chi.Router) {
		r.Use(apimiddleware.JWTAuth(cfg.JWTSecret))
		r.Handle("/positions/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/pnl/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/orders/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/ws/positions/*", handler.NewWebSocketProxy(cfg.ExchangeURL))
	})

	log.Printf("Server starting on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
