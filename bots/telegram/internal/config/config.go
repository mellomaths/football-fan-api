// Package config loads bot and API settings from the environment.
package config

import (
	"errors"
	"os"
	"strings"
)

// Config holds runtime configuration for the Telegram bot.
type Config struct {
	TelegramToken string
	APIBaseURL    string
	APIKey        string
	// NotifyTZ is an IANA timezone name for match digests (e.g. America/Sao_Paulo). Defaults to UTC.
	NotifyTZ string
}

// Load reads required environment variables.
func Load() (Config, error) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	base := strings.TrimSpace(os.Getenv("API_BASE_URL"))
	if base == "" {
		return Config{}, errors.New("API_BASE_URL is required")
	}
	base = strings.TrimRight(base, "/")
	key := strings.TrimSpace(os.Getenv("API_INTERNAL_KEY"))
	if key == "" {
		return Config{}, errors.New("API_INTERNAL_KEY is required")
	}
	tz := strings.TrimSpace(os.Getenv("NOTIFY_TZ"))
	if tz == "" {
		tz = "UTC"
	}
	return Config{
		TelegramToken: token,
		APIBaseURL:    base,
		APIKey:        key,
		NotifyTZ:      tz,
	}, nil
}
