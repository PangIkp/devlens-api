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
	t.Setenv("SYNC_WORKER_BATCH_SIZE", "20")
	t.Setenv("SYNC_WORKER_CONCURRENCY", "4")
	t.Setenv("SYNC_JOB_TIMEOUT", "2m")
	t.Setenv("WEBHOOK_RETRY_INTERVAL", "15s")
	t.Setenv("WEBHOOK_RETRY_BATCH_SIZE", "8")
	t.Setenv("WEBHOOK_RETRY_CONCURRENCY", "3")
	t.Setenv("WEBHOOK_RETRY_TIMEOUT", "45s")
	t.Setenv("GITHUB_RATE_LIMIT_MIN_REMAINING", "75")
	t.Setenv("METRICS_DEFAULT_DAY_TYPE", "business")
	t.Setenv("METRICS_HOTSPOT_WEIGHT_COMMITS", "2")
	t.Setenv("METRICS_HOTSPOT_WEIGHT_ADDITIONS", "0.5")
	t.Setenv("METRICS_HOTSPOT_WEIGHT_DELETIONS", "1.5")
	t.Setenv("INSIGHTS_LARGE_PR_FILES_THRESHOLD", "30")
	t.Setenv("INSIGHTS_SLOW_REVIEW_WAIT_HOURS_THRESHOLD", "36")
	t.Setenv("INSIGHTS_DEPLOYMENT_FAILURE_RATE_THRESHOLD", "0.4")
	t.Setenv("INSIGHTS_AUTO_REOPEN_MINIMUM_SEVERITY", "medium")
	t.Setenv("INSIGHTS_DEDUPLICATE_VERSION", "2")
	t.Setenv("ANALYTICS_RAW_RETENTION_DAYS", "120")
	t.Setenv("ANALYTICS_AGGREGATE_RETENTION_DAYS", "365")
	t.Setenv("WEBHOOK_PAYLOAD_RETENTION_DAYS", "14")
	t.Setenv("SOFT_DELETED_ORGANIZATION_RETENTION_DAYS", "21")
	t.Setenv("DISCONNECTED_INSTALLATION_RETENTION_DAYS", "28")
	t.Setenv("CLICKHOUSE_INSERT_BATCH_SIZE", "250")

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
	if cfg.Sync.WorkerBatchSize != 20 {
		t.Fatalf("unexpected worker batch size %d", cfg.Sync.WorkerBatchSize)
	}
	if cfg.Sync.WorkerConcurrency != 4 {
		t.Fatalf("unexpected worker concurrency %d", cfg.Sync.WorkerConcurrency)
	}
	if cfg.Sync.JobTimeout != 2*time.Minute {
		t.Fatalf("unexpected sync job timeout %s", cfg.Sync.JobTimeout)
	}
	if cfg.Sync.WebhookRetryBatchSize != 8 {
		t.Fatalf("unexpected webhook retry batch size %d", cfg.Sync.WebhookRetryBatchSize)
	}
	if cfg.Sync.WebhookRetryConcurrency != 3 {
		t.Fatalf("unexpected webhook retry concurrency %d", cfg.Sync.WebhookRetryConcurrency)
	}
	if cfg.Sync.WebhookRetryTimeout != 45*time.Second {
		t.Fatalf("unexpected webhook retry timeout %s", cfg.Sync.WebhookRetryTimeout)
	}
	if cfg.Sync.GitHubRateLimitRemaining != 75 {
		t.Fatalf("unexpected github rate limit remaining threshold %d", cfg.Sync.GitHubRateLimitRemaining)
	}
	if cfg.Metrics.DefaultDayType != "business" {
		t.Fatalf("unexpected metrics default day type %q", cfg.Metrics.DefaultDayType)
	}
	if cfg.Metrics.HotspotCommitWeight != 2 {
		t.Fatalf("unexpected hotspot commit weight %v", cfg.Metrics.HotspotCommitWeight)
	}
	if cfg.Metrics.HotspotAdditionsWeight != 0.5 {
		t.Fatalf("unexpected hotspot additions weight %v", cfg.Metrics.HotspotAdditionsWeight)
	}
	if cfg.Metrics.HotspotDeletionsWeight != 1.5 {
		t.Fatalf("unexpected hotspot deletions weight %v", cfg.Metrics.HotspotDeletionsWeight)
	}
	if cfg.Insights.LargePRFilesThreshold != 30 {
		t.Fatalf("unexpected large pr files threshold %d", cfg.Insights.LargePRFilesThreshold)
	}
	if cfg.Insights.SlowReviewWaitHoursThreshold != 36 {
		t.Fatalf("unexpected slow review wait hours threshold %v", cfg.Insights.SlowReviewWaitHoursThreshold)
	}
	if cfg.Insights.DeploymentFailureRateThreshold != 0.4 {
		t.Fatalf("unexpected deployment failure rate threshold %v", cfg.Insights.DeploymentFailureRateThreshold)
	}
	if cfg.Insights.AutoReopenMinimumSeverity != "medium" {
		t.Fatalf("unexpected auto reopen minimum severity %q", cfg.Insights.AutoReopenMinimumSeverity)
	}
	if cfg.Insights.DeduplicateVersion != 2 {
		t.Fatalf("unexpected deduplicate version %d", cfg.Insights.DeduplicateVersion)
	}
	if cfg.DataLifecycle.AnalyticsRawRetentionDays != 120 {
		t.Fatalf("unexpected analytics raw retention days %d", cfg.DataLifecycle.AnalyticsRawRetentionDays)
	}
	if cfg.DataLifecycle.WebhookPayloadRetentionDays != 14 {
		t.Fatalf("unexpected webhook payload retention days %d", cfg.DataLifecycle.WebhookPayloadRetentionDays)
	}
	if cfg.DataLifecycle.ClickHouseInsertBatchSize != 250 {
		t.Fatalf("unexpected clickhouse insert batch size %d", cfg.DataLifecycle.ClickHouseInsertBatchSize)
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

func TestLoadRejectsInvalidWorkerConcurrency(t *testing.T) {
	t.Setenv("SYNC_WORKER_CONCURRENCY", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidMetricsDefaultDayType(t *testing.T) {
	t.Setenv("METRICS_DEFAULT_DAY_TYPE", "weird")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsAllZeroHotspotWeights(t *testing.T) {
	t.Setenv("METRICS_HOTSPOT_WEIGHT_COMMITS", "0")
	t.Setenv("METRICS_HOTSPOT_WEIGHT_ADDITIONS", "0")
	t.Setenv("METRICS_HOTSPOT_WEIGHT_DELETIONS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidInsightAutoReopenSeverity(t *testing.T) {
	t.Setenv("INSIGHTS_AUTO_REOPEN_MINIMUM_SEVERITY", "urgent")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidAnalyticsRetentionWindow(t *testing.T) {
	t.Setenv("ANALYTICS_RAW_RETENTION_DAYS", "365")
	t.Setenv("ANALYTICS_AGGREGATE_RETENTION_DAYS", "180")

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

func TestLoadAllowsEmptyClickHouseDSN(t *testing.T) {
	t.Setenv("CLICKHOUSE_DSN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.ClickHouse.DSN != "" {
		t.Fatalf("expected empty clickhouse dsn, got %q", cfg.ClickHouse.DSN)
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

func TestLoadTracingConfigFromEnv(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_SERVICE_NAME", "devlens-api-local")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")
	t.Setenv("OTEL_TRACE_SAMPLE_RATIO", "0.5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !cfg.Tracing.Enabled {
		t.Fatal("expected tracing to be enabled")
	}
	if cfg.Tracing.ServiceName != "devlens-api-local" {
		t.Fatalf("unexpected tracing service name %q", cfg.Tracing.ServiceName)
	}
	if cfg.Tracing.ExporterEndpoint != "localhost:4318" {
		t.Fatalf("unexpected tracing endpoint %q", cfg.Tracing.ExporterEndpoint)
	}
	if cfg.Tracing.Insecure {
		t.Fatal("expected tracing insecure flag to be false")
	}
	if cfg.Tracing.SampleRatio != 0.5 {
		t.Fatalf("unexpected tracing sample ratio %v", cfg.Tracing.SampleRatio)
	}
}
