// Package config loads the Go agent's runtime configuration from
// environment variables.
package config

import (
	"os"
	"strings"
	"time"
)

// Config holds the agent's runtime settings.
type Config struct {
	// Port is the TCP port the REST API listens on.
	Port string
	// LogLevel is the minimum level logged (debug, info, warn, error).
	LogLevel string
	// PostgresDSN is the connection string for the Postgres/TimescaleDB
	// instance the agent writes metrics and anomalies to. Required —
	// left empty if unset, which surfaces as a connection error at
	// startup rather than being validated here.
	PostgresDSN string
	// RabbitMQURL is the AMQP URL the agent publishes raw metrics to.
	// Empty disables publishing; the agent still polls and writes to
	// Postgres.
	RabbitMQURL string
	// WatchNamespaces restricts polling to these namespaces. Empty
	// means watch all namespaces.
	WatchNamespaces []string
	// PollInterval is how often the agent polls the Kubernetes APIs.
	PollInterval time.Duration
	// CORSOrigin is the origin allowed to make cross-origin requests to
	// the REST API (the dashboard's dev/prod origin).
	CORSOrigin string
}

// Purpose: reads configuration from environment variables, falling
// back to defaults for anything unset.
// Params: none.
// Returns: a populated Config.
func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		PostgresDSN:     getEnv("POSTGRES_DSN", ""),
		RabbitMQURL:     getEnv("RABBITMQ_URL", ""),
		WatchNamespaces: parseNamespaces(getEnv("WATCH_NAMESPACES", "")),
		PollInterval:    parseDuration(getEnv("POLL_INTERVAL", "15s"), 15*time.Second),
		CORSOrigin:      getEnv("CORS_ORIGIN", "http://localhost:5173"),
	}
}

// Purpose: returns the named environment variable's value, or
// fallback if it's unset or empty.
// Params:
//   - key: the environment variable name.
//   - fallback: the value to return if key is unset or empty.
//
// Returns: the environment variable's value, or fallback.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Purpose: splits a comma-separated namespace list, trimming
// whitespace and dropping empty entries; nil means watch all.
// Params:
//   - raw: the raw comma-separated namespace list.
//
// Returns: the parsed namespace list, or nil if raw is empty.
func parseNamespaces(raw string) []string {
	if raw == "" {
		return nil
	}
	var namespaces []string
	for _, ns := range strings.Split(raw, ",") {
		ns = strings.TrimSpace(ns)
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces
}

// Purpose: parses a duration string, falling back to the given
// default if it's empty or invalid.
// Params:
//   - raw: the raw duration string (e.g. "15s").
//   - fallback: the value to return if raw is empty or invalid.
//
// Returns: the parsed duration, or fallback.
func parseDuration(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
