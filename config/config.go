package config

import (
	"os"
)

type Config struct {
	Port          string
	JWTSecret     string
	MarketDataURL string
	ExchangeURL   string
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
		exchangeURL = "http://host.docker.internal:8081"
	}

	return &Config{
		Port:          port,
		JWTSecret:     os.Getenv("JWT_SECRET"),
		MarketDataURL: marketDataURL,
		ExchangeURL:   exchangeURL,
	}
}
