package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Vuate/api-gateway/config"
	"github.com/Vuate/api-gateway/internal/handler"
	apimiddleware "github.com/Vuate/api-gateway/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"
)

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return def
}

func main() {
	cfg := config.Load()

	// Group A: high-frequency market data (quotes, history, ohlcv, compare)
	rlA := apimiddleware.NewRateLimiter(cfg.RedisURL, "groupA", getEnvInt("RATE_LIMIT_GROUP_A", 30), time.Second)
	// Group B: order book and analytics endpoints
	rlB := apimiddleware.NewRateLimiter(cfg.RedisURL, "groupB", getEnvInt("RATE_LIMIT_GROUP_B", 5), time.Second)
	// Group C: expensive / low-frequency endpoints (news, whale-alerts, etc.)
	rlC := apimiddleware.NewRateLimiter(cfg.RedisURL, "groupC", getEnvInt("RATE_LIMIT_GROUP_C", 2), time.Second)
	// Auth: brute force koruması — login/register dakika bazlı limitli
	rlAuthLogin := apimiddleware.NewRateLimiter(cfg.RedisURL, "auth-login", getEnvInt("RATE_LIMIT_AUTH_LOGIN", 10), time.Minute)
	rlAuthRegister := apimiddleware.NewRateLimiter(cfg.RedisURL, "auth-register", getEnvInt("RATE_LIMIT_AUTH_REGISTER", 5), time.Minute)

	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000"
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(apimiddleware.Metrics)
	r.Use(apimiddleware.RequestLogger)

	r.Get("/health", handler.Health(cfg))
	r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.yaml")))
	r.Get("/swagger/doc.yaml", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, "docs/swagger.yaml")
	})
	r.Handle("/metrics", promhttp.Handler())

	marketDataCB := apimiddleware.NewCircuitBreaker("market-data")
	exchangeCB := apimiddleware.NewCircuitBreaker("exchange")
	authCB := apimiddleware.NewCircuitBreaker("auth")

	marketDataTimeout := getEnvDuration("TIMEOUT_MARKET_DATA", 5*time.Second)
	exchangeTimeout := getEnvDuration("TIMEOUT_EXCHANGE", 10*time.Second)
	authTimeout := getEnvDuration("TIMEOUT_AUTH", 10*time.Second)

	// Auth — JWT gerekmez, endpoint bazlı rate limit (brute force koruması)
	r.Group(func(r chi.Router) {
		r.Use(apimiddleware.TimeoutMiddleware(authTimeout))
		r.With(rlAuthLogin.Middleware).Handle("/api/v1/auth/login", authCB.Wrap(handler.NewProxy(cfg.AuthURL)))
		r.With(rlAuthRegister.Middleware).Handle("/api/v1/auth/register", authCB.Wrap(handler.NewProxy(cfg.AuthURL)))
		r.Handle("/api/v1/auth/*", authCB.Wrap(handler.NewProxy(cfg.AuthURL)))
	})

	// Group A — 30 RPS: high-frequency market data
	r.Group(func(r chi.Router) {
		r.Use(rlA.Middleware)
		r.Use(apimiddleware.TimeoutMiddleware(marketDataTimeout))
		r.Handle("/api/v1/quotes/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/history/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/ohlcv/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/compare/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	})

	// Group B — 5 RPS: order book and analytics
	r.Group(func(r chi.Router) {
		r.Use(rlB.Middleware)
		r.Use(apimiddleware.TimeoutMiddleware(marketDataTimeout))
		r.Handle("/api/v1/orderbook/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/spread/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/funding/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/funding-rate/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/slippage/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/liquidity/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/efficiency/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/rsi/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	})

	// Group C — 2 RPS: expensive / low-frequency endpoints
	r.Group(func(r chi.Router) {
		r.Use(rlC.Middleware)
		r.Use(apimiddleware.TimeoutMiddleware(marketDataTimeout))
		r.Handle("/api/v1/whale-alerts", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/wallet/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/news", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/ico-calendar", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/etf-flows", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/fees", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/all-in-cost/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
		r.Handle("/api/v1/token-flow/*", marketDataCB.Wrap(handler.NewProxy(cfg.MarketDataURL)))
	})

	// WebSocket — circuit breaker yok, rate limit yok, timeout yok
	r.Handle("/ws", handler.NewWebSocketProxy(cfg.MarketDataURL))
	r.Handle("/ws/quotes/*", handler.NewWebSocketProxy(cfg.MarketDataURL))
	r.Handle("/ws/orderbook", handler.NewWebSocketProxy(cfg.MarketDataURL))

	// Protected — JWT zorunlu
	r.Group(func(r chi.Router) {
		r.Use(apimiddleware.JWTAuthWithRedis(cfg.JWTSecret, cfg.RedisURL))
		r.Use(apimiddleware.TimeoutMiddleware(exchangeTimeout))
		r.Handle("/positions/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/pnl/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/orders", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/orders/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/trades/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/dca/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/risk/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/apikeys/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/apikeys", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/alerts", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/alerts/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/position/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/portfolio/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/users/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/performance", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/staking", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/staking/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/stacks", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/dashboard", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/dashboard/*", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))
		r.Handle("/api/v1/futures/positions", exchangeCB.Wrap(handler.NewProxy(cfg.ExchangeURL)))

		// WebSocket — forwarded after JWT validation
		r.Handle("/ws/positions/*", handler.NewWebSocketProxy(cfg.ExchangeURL))
		r.Handle("/api/v1/ws", handler.NewWebSocketProxy(cfg.ExchangeURL))
		r.Handle("/api/v1/ws/*", handler.NewWebSocketProxy(cfg.ExchangeURL))
	})

	log.Printf("Server starting on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
