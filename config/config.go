package config

import (
	"os"
)

type Config struct {
	Port      string
	JWTSecret string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	return &Config{
		Port:      port,
		JWTSecret: os.Getenv("JWT_SECRET"),
	}
}
