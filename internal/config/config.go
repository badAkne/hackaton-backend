package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
}

func Load() Config {
	return Config{
		Port:        get("PORT", "8080"),
		DatabaseURL: get("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/inventory?sslmode=disable"),
	}
}

func get(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
