package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("WATCH_NAMESPACES", "")
	t.Setenv("POLL_INTERVAL", "")
	t.Setenv("CORS_ORIGIN", "")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.PostgresDSN != "" {
		t.Errorf("PostgresDSN = %q, want empty", cfg.PostgresDSN)
	}
	if cfg.WatchNamespaces != nil {
		t.Errorf("WatchNamespaces = %v, want nil", cfg.WatchNamespaces)
	}
	if cfg.PollInterval != 15*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 15*time.Second)
	}
	if cfg.CORSOrigin != "http://localhost:5173" {
		t.Errorf("CORSOrigin = %q, want %q", cfg.CORSOrigin, "http://localhost:5173")
	}
}

func TestLoad_CORSOriginCustom(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "https://dashboard.example.com")

	cfg := Load()

	if cfg.CORSOrigin != "https://dashboard.example.com" {
		t.Errorf("CORSOrigin = %q, want %q", cfg.CORSOrigin, "https://dashboard.example.com")
	}
}

func TestLoad_WatchNamespacesParsesAndTrims(t *testing.T) {
	t.Setenv("WATCH_NAMESPACES", " default , podsentinel-demo ,,staging")

	cfg := Load()

	want := []string{"default", "podsentinel-demo", "staging"}
	if len(cfg.WatchNamespaces) != len(want) {
		t.Fatalf("WatchNamespaces = %v, want %v", cfg.WatchNamespaces, want)
	}
	for i, ns := range want {
		if cfg.WatchNamespaces[i] != ns {
			t.Errorf("WatchNamespaces[%d] = %q, want %q", i, cfg.WatchNamespaces[i], ns)
		}
	}
}

func TestLoad_PollIntervalCustom(t *testing.T) {
	t.Setenv("POLL_INTERVAL", "30s")

	cfg := Load()

	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 30*time.Second)
	}
}

func TestLoad_PollIntervalInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("POLL_INTERVAL", "not-a-duration")

	cfg := Load()

	if cfg.PollInterval != 15*time.Second {
		t.Errorf("PollInterval = %v, want default %v", cfg.PollInterval, 15*time.Second)
	}
}

func TestLoad_PostgresDSNPassthrough(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/podsentinel")

	cfg := Load()

	want := "postgres://user:pass@localhost:5432/podsentinel"
	if cfg.PostgresDSN != want {
		t.Errorf("PostgresDSN = %q, want %q", cfg.PostgresDSN, want)
	}
}
