package config

import (
	"os"
)

type Config struct {
	Port          string
	JWTSecret     string
	MarketDataURL string
	ExchangeURL   string
	AuthURL       string
	RedisURL      string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	marketDataURL := os.Getenv("MARKET_DATA_URL")
	if marketDataURL == "" {
		marketDataURL = "https://levi-overdainty-complimentingly.ngrok-free.dev"
	}

	exchangeURL := os.Getenv("EXCHANGE_URL")
	if exchangeURL == "" {
		exchangeURL = "https://contextured-tora-nontribally.ngrok-free.dev"
	}

	authURL := os.Getenv("AUTH_URL")
	if authURL == "" {
		authURL = "http://auth-service:8082"
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis:6379"
	}

	return &Config{
		Port:          port,
		JWTSecret:     os.Getenv("JWT_SECRET"),
		MarketDataURL: marketDataURL,
		ExchangeURL:   exchangeURL,
		AuthURL:       authURL,
		RedisURL:      redisURL,
	}
}
