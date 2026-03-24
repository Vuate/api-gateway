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

	r.Get("/health", handler.Health)

	// Public — JWT gerekmez
	r.Handle("/api/v1/quotes/*", handler.NewProxy("https://levi-overdainty-complimentingly.ngrok-free.dev"))
	r.Handle("/ws/quotes/*", handler.NewWebSocketProxy("https://levi-overdainty-complimentingly.ngrok-free.dev"))

	// Protected — JWT zorunlu
	r.Group(func(r chi.Router) {
		r.Use(apimiddleware.JWTAuth(cfg.JWTSecret))
		r.Handle("/positions/*", handler.NewProxy("http://host.docker.internal:8081"))
		r.Handle("/api/v1/pnl/*", handler.NewProxy("http://host.docker.internal:8081"))
		r.Handle("/api/v1/orders/*", handler.NewProxy("http://host.docker.internal:8081"))
		r.Handle("/ws/positions/*", handler.NewWebSocketProxy("http://host.docker.internal:8081"))
	})

	log.Printf("Server starting on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
