package config

import (
	"os"
	"strconv"
)

type Config struct {
	WebGuardCoreAPIKey string
	WebGuardCoreAPIURL string
	WebGuardLocation   string

	QueueDefaultWorkers int

	Address string
}

func FromEnv() Config {
	port := env("PORT", "8080")
	return Config{
		WebGuardCoreAPIKey: env("WEBGUARD_CORE_API_KEY", ""),
		WebGuardCoreAPIURL: env("WEBGUARD_CORE_API_URL", ""),
		WebGuardLocation:   env("WEBGUARD_LOCATION", ""),

		QueueDefaultWorkers: envInt("QUEUE_DEFAULT_WORKERS", 3),

		Address: env("BIND_ADDRESS", ":"+port),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
