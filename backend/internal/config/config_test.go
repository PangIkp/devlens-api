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
}

func TestLoadRejectsInvalidGitHubBackoffWindow(t *testing.T) {
	t.Setenv("GITHUB_INITIAL_BACKOFF", "5s")
	t.Setenv("GITHUB_MAX_BACKOFF", "1s")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}
