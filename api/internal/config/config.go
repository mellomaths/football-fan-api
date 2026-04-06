// Package config loads process configuration from the environment.
package config

import (
	"errors"
	"os"
)

// Config holds process configuration from the environment.
type Config struct {
	DatabaseURL string
	HTTPAddr    string
}

// Load reads required configuration. DATABASE_URL must be set.
func Load() (Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, errors.New("database_url is required")
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return Config{
		DatabaseURL: dbURL,
		HTTPAddr:    addr,
	}, nil
}
