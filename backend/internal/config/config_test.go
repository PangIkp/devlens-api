package config

import (
	"testing"
	"time"
)

func TestLoadGitHubConfigFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	t.Setenv("GITHUB_API_BASE_URL", "https://ghe.example.com/api/v3")
	t.Setenv("GITHUB_USER_AGENT", "devlens-test")
	t.Setenv("GITHUB_HTTP_TIMEOUT", "15s")
	t.Setenv("GITHUB_MAX_RETRIES", "4")
	t.Setenv("GITHUB_INITIAL_BACKOFF", "250ms")
	t.Setenv("GITHUB_MAX_BACKOFF", "3s")
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_INSTALL_URL", "https://github.com/apps/devlens/installations/new")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----")
	t.Setenv("WEBHOOK_RETRY_INTERVAL", "15s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.GitHub.Token != "secret-token" {
		t.Fatalf("unexpected github token %q", cfg.GitHub.Token)
	}
	if cfg.GitHub.BaseURL != "https://ghe.example.com/api/v3" {
		t.Fatalf("unexpected github base url %q", cfg.GitHub.BaseURL)
	}
	if cfg.GitHub.UserAgent != "devlens-test" {
		t.Fatalf("unexpected github user agent %q", cfg.GitHub.UserAgent)
	}
	if cfg.GitHub.HTTPTimeout != 15*time.Second {
		t.Fatalf("unexpected github timeout %s", cfg.GitHub.HTTPTimeout)
	}
	if cfg.GitHub.MaxRetries != 4 {
		t.Fatalf("unexpected github max retries %d", cfg.GitHub.MaxRetries)
	}
	if cfg.GitHub.InitialBackoff != 250*time.Millisecond {
		t.Fatalf("unexpected github initial backoff %s", cfg.GitHub.InitialBackoff)
	}
	if cfg.GitHub.MaxBackoff != 3*time.Second {
		t.Fatalf("unexpected github max backoff %s", cfg.GitHub.MaxBackoff)
	}
	if cfg.GitHub.App.AppID != 12345 {
		t.Fatalf("unexpected github app id %d", cfg.GitHub.App.AppID)
	}
	if cfg.GitHub.App.InstallURL != "https://github.com/apps/devlens/installations/new" {
		t.Fatalf("unexpected github app install url %q", cfg.GitHub.App.InstallURL)
	}
	if cfg.GitHub.App.PrivateKey == "" {
		t.Fatal("expected github app private key to be loaded")
	}
	if cfg.Sync.WebhookRetryInterval != 15*time.Second {
		t.Fatalf("unexpected webhook retry interval %s", cfg.Sync.WebhookRetryInterval)
	}
}

func TestLoadRejectsInvalidGitHubBackoffWindow(t *testing.T) {
	t.Setenv("GITHUB_INITIAL_BACKOFF", "5s")
	t.Setenv("GITHUB_MAX_BACKOFF", "1s")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidWebhookRetryInterval(t *testing.T) {
	t.Setenv("WEBHOOK_RETRY_INTERVAL", "0s")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadClickHouseTimeoutFromEnv(t *testing.T) {
	t.Setenv("CLICKHOUSE_HTTP_TIMEOUT", "7s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.ClickHouse.Timeout != 7*time.Second {
		t.Fatalf("unexpected clickhouse timeout %s", cfg.ClickHouse.Timeout)
	}
}

func TestLoadCORSAllowedOriginsFromEnv(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, https://app.devlens.dev")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(cfg.HTTP.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 origins, got %d", len(cfg.HTTP.AllowedOrigins))
	}
	if cfg.HTTP.AllowedOrigins[0] != "http://localhost:3000" {
		t.Fatalf("unexpected first origin %q", cfg.HTTP.AllowedOrigins[0])
	}
	if cfg.HTTP.AllowedOrigins[1] != "https://app.devlens.dev" {
		t.Fatalf("unexpected second origin %q", cfg.HTTP.AllowedOrigins[1])
	}
}

func TestLoadRateLimitFromEnv(t *testing.T) {
	t.Setenv("RATE_LIMIT_REQUESTS", "30")
	t.Setenv("RATE_LIMIT_WINDOW", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.HTTP.RateLimit.Requests != 30 {
		t.Fatalf("unexpected rate limit requests %d", cfg.HTTP.RateLimit.Requests)
	}
	if cfg.HTTP.RateLimit.Window != 30*time.Second {
		t.Fatalf("unexpected rate limit window %s", cfg.HTTP.RateLimit.Window)
	}
}
