package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv     string
	LogLevel   slog.Level
	HTTP       HTTPConfig
	Postgres   PostgresConfig
	ClickHouse ClickHouseConfig
	GitHub     GitHubConfig
	NATS       NATSConfig
	Sync       SyncConfig
	Metrics    MetricsConfig
	Tracing    TracingConfig
}

type HTTPConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	AllowedOrigins  []string
	RateLimit       RateLimitConfig
}

type RateLimitConfig struct {
	Requests int
	Window   time.Duration
}

type PostgresConfig struct {
	Host              string
	Port              string
	User              string
	Password          string
	Database          string
	SSLMode           string
	DSN               string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
}

type ClickHouseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	DSN      string
	Timeout  time.Duration
}

type GitHubConfig struct {
	Token          string
	BaseURL        string
	UserAgent      string
	WebhookSecret  string
	HTTPTimeout    time.Duration
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	App            GitHubAppConfig
}

type GitHubAppConfig struct {
	AppID      int64
	InstallURL string
	PrivateKey string
}

type NATSConfig struct {
	URL string
}

type SyncConfig struct {
	WorkerPollInterval       time.Duration
	WorkerBatchSize          int
	WorkerConcurrency        int
	JobTimeout               time.Duration
	WebhookRetryInterval     time.Duration
	WebhookRetryBatchSize    int
	WebhookRetryConcurrency  int
	WebhookRetryTimeout      time.Duration
	GitHubRateLimitRemaining int
}

type TracingConfig struct {
	Enabled          bool
	ServiceName      string
	ExporterEndpoint string
	Insecure         bool
	SampleRatio      float64
}

type MetricsConfig struct {
	DefaultDayType         string
	HotspotCommitWeight    float64
	HotspotAdditionsWeight float64
	HotspotDeletionsWeight float64
}

func Load() (Config, error) {
	logLevel, err := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := getDuration("HTTP_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := getDuration("HTTP_IDLE_TIMEOUT", time.Minute)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	rateLimitWindow, err := getDuration("RATE_LIMIT_WINDOW", time.Minute)
	if err != nil {
		return Config{}, err
	}
	rateLimitRequests, err := getInt("RATE_LIMIT_REQUESTS", 120)
	if err != nil {
		return Config{}, err
	}

	pgMaxConnLifetime, err := getDuration("POSTGRES_MAX_CONN_LIFETIME", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	pgMaxConnIdleTime, err := getDuration("POSTGRES_MAX_CONN_IDLE_TIME", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}

	pgHealthCheckPeriod, err := getDuration("POSTGRES_HEALTH_CHECK_PERIOD", time.Minute)
	if err != nil {
		return Config{}, err
	}

	pgConnectTimeout, err := getDuration("POSTGRES_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	clickhouseTimeout, err := getDuration("CLICKHOUSE_HTTP_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	pgMaxConns, err := getInt32("POSTGRES_MAX_CONNS", 10)
	if err != nil {
		return Config{}, err
	}

	pgMinConns, err := getInt32("POSTGRES_MIN_CONNS", 0)
	if err != nil {
		return Config{}, err
	}

	githubHTTPTimeout, err := getDuration("GITHUB_HTTP_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	githubInitialBackoff, err := getDuration("GITHUB_INITIAL_BACKOFF", 500*time.Millisecond)
	if err != nil {
		return Config{}, err
	}

	githubMaxBackoff, err := getDuration("GITHUB_MAX_BACKOFF", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	githubMaxRetries, err := getInt("GITHUB_MAX_RETRIES", 3)
	if err != nil {
		return Config{}, err
	}

	syncWorkerPollInterval, err := getDuration("SYNC_WORKER_POLL_INTERVAL", 2*time.Second)
	if err != nil {
		return Config{}, err
	}
	syncWorkerBatchSize, err := getInt("SYNC_WORKER_BATCH_SIZE", 10)
	if err != nil {
		return Config{}, err
	}
	syncWorkerConcurrency, err := getInt("SYNC_WORKER_CONCURRENCY", 2)
	if err != nil {
		return Config{}, err
	}
	syncJobTimeout, err := getDuration("SYNC_JOB_TIMEOUT", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	webhookRetryInterval, err := getDuration("WEBHOOK_RETRY_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	webhookRetryBatchSize, err := getInt("WEBHOOK_RETRY_BATCH_SIZE", 10)
	if err != nil {
		return Config{}, err
	}
	webhookRetryConcurrency, err := getInt("WEBHOOK_RETRY_CONCURRENCY", 2)
	if err != nil {
		return Config{}, err
	}
	webhookRetryTimeout, err := getDuration("WEBHOOK_RETRY_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	githubRateLimitRemaining, err := getInt("GITHUB_RATE_LIMIT_MIN_REMAINING", 100)
	if err != nil {
		return Config{}, err
	}
	traceSampleRatio, err := getFloat64("OTEL_TRACE_SAMPLE_RATIO", 1)
	if err != nil {
		return Config{}, err
	}
	metricsHotspotCommitWeight, err := getFloat64("METRICS_HOTSPOT_WEIGHT_COMMITS", 1)
	if err != nil {
		return Config{}, err
	}
	metricsHotspotAdditionsWeight, err := getFloat64("METRICS_HOTSPOT_WEIGHT_ADDITIONS", 1)
	if err != nil {
		return Config{}, err
	}
	metricsHotspotDeletionsWeight, err := getFloat64("METRICS_HOTSPOT_WEIGHT_DELETIONS", 1)
	if err != nil {
		return Config{}, err
	}

	httpCfg := HTTPConfig{
		Addr:            getEnv("HTTP_ADDR", ":8080"),
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		IdleTimeout:     idleTimeout,
		ShutdownTimeout: shutdownTimeout,
		AllowedOrigins:  splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		RateLimit: RateLimitConfig{
			Requests: rateLimitRequests,
			Window:   rateLimitWindow,
		},
	}

	cfg := Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		LogLevel: logLevel,
		HTTP:     httpCfg,
		Postgres: PostgresConfig{
			Host:              getEnv("POSTGRES_HOST", "localhost"),
			Port:              getEnv("POSTGRES_PORT", "5432"),
			User:              getEnv("POSTGRES_USER", "devlens"),
			Password:          getEnv("POSTGRES_PASSWORD", "devlens"),
			Database:          getEnv("POSTGRES_DB", "devlens"),
			SSLMode:           getEnv("POSTGRES_SSLMODE", "disable"),
			DSN:               strings.TrimSpace(getEnv("POSTGRES_DSN", "")),
			MaxConns:          pgMaxConns,
			MinConns:          pgMinConns,
			MaxConnLifetime:   pgMaxConnLifetime,
			MaxConnIdleTime:   pgMaxConnIdleTime,
			HealthCheckPeriod: pgHealthCheckPeriod,
			ConnectTimeout:    pgConnectTimeout,
		},
		ClickHouse: ClickHouseConfig{
			Host:     getEnv("CLICKHOUSE_HOST", "localhost"),
			Port:     getEnv("CLICKHOUSE_PORT", "8123"),
			User:     getEnv("CLICKHOUSE_USER", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
			Database: getEnv("CLICKHOUSE_DATABASE", "devlens"),
			DSN:      getEnv("CLICKHOUSE_DSN", "http://localhost:8123"),
			Timeout:  clickhouseTimeout,
		},
		GitHub: GitHubConfig{
			Token:          strings.TrimSpace(getEnv("GITHUB_TOKEN", "")),
			BaseURL:        strings.TrimSpace(getEnv("GITHUB_API_BASE_URL", "https://api.github.com")),
			UserAgent:      strings.TrimSpace(getEnv("GITHUB_USER_AGENT", "devlens-api")),
			WebhookSecret:  getEnv("GITHUB_WEBHOOK_SECRET", ""),
			HTTPTimeout:    githubHTTPTimeout,
			MaxRetries:     githubMaxRetries,
			InitialBackoff: githubInitialBackoff,
			MaxBackoff:     githubMaxBackoff,
			App: GitHubAppConfig{
				AppID:      getInt64Value("GITHUB_APP_ID"),
				InstallURL: strings.TrimSpace(getEnv("GITHUB_APP_INSTALL_URL", "")),
				PrivateKey: getEnv("GITHUB_APP_PRIVATE_KEY", ""),
			},
		},
		NATS: NATSConfig{
			URL: getEnv("NATS_URL", "nats://localhost:4222"),
		},
		Sync: SyncConfig{
			WorkerPollInterval:       syncWorkerPollInterval,
			WorkerBatchSize:          syncWorkerBatchSize,
			WorkerConcurrency:        syncWorkerConcurrency,
			JobTimeout:               syncJobTimeout,
			WebhookRetryInterval:     webhookRetryInterval,
			WebhookRetryBatchSize:    webhookRetryBatchSize,
			WebhookRetryConcurrency:  webhookRetryConcurrency,
			WebhookRetryTimeout:      webhookRetryTimeout,
			GitHubRateLimitRemaining: githubRateLimitRemaining,
		},
		Metrics: MetricsConfig{
			DefaultDayType:         strings.TrimSpace(strings.ToLower(getEnv("METRICS_DEFAULT_DAY_TYPE", "calendar"))),
			HotspotCommitWeight:    metricsHotspotCommitWeight,
			HotspotAdditionsWeight: metricsHotspotAdditionsWeight,
			HotspotDeletionsWeight: metricsHotspotDeletionsWeight,
		},
		Tracing: TracingConfig{
			Enabled:          getBool("OTEL_ENABLED", false),
			ServiceName:      strings.TrimSpace(getEnv("OTEL_SERVICE_NAME", "devlens-api")),
			ExporterEndpoint: strings.TrimSpace(getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "")),
			Insecure:         getBool("OTEL_EXPORTER_OTLP_INSECURE", true),
			SampleRatio:      traceSampleRatio,
		},
	}

	if strings.TrimSpace(cfg.HTTP.Addr) == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if cfg.HTTP.RateLimit.Requests < 0 {
		return Config{}, fmt.Errorf("RATE_LIMIT_REQUESTS must be greater than or equal to 0")
	}
	if cfg.Tracing.SampleRatio <= 0 || cfg.Tracing.SampleRatio > 1 {
		return Config{}, fmt.Errorf("OTEL_TRACE_SAMPLE_RATIO must be greater than 0 and less than or equal to 1")
	}
	if cfg.HTTP.RateLimit.Requests > 0 && cfg.HTTP.RateLimit.Window <= 0 {
		return Config{}, fmt.Errorf("RATE_LIMIT_WINDOW must be greater than 0 when rate limiting is enabled")
	}

	if strings.TrimSpace(cfg.Postgres.Host) == "" && strings.TrimSpace(cfg.Postgres.DSN) == "" {
		return Config{}, fmt.Errorf("POSTGRES_HOST or POSTGRES_DSN must not be empty")
	}

	if cfg.Postgres.MaxConns < 1 {
		return Config{}, fmt.Errorf("POSTGRES_MAX_CONNS must be greater than 0")
	}

	if cfg.Postgres.MinConns < 0 {
		return Config{}, fmt.Errorf("POSTGRES_MIN_CONNS must be greater than or equal to 0")
	}

	if cfg.Postgres.MinConns > cfg.Postgres.MaxConns {
		return Config{}, fmt.Errorf("POSTGRES_MIN_CONNS must be less than or equal to POSTGRES_MAX_CONNS")
	}

	if _, err := url.ParseRequestURI(cfg.GitHub.BaseURL); err != nil {
		return Config{}, fmt.Errorf("GITHUB_API_BASE_URL must be a valid URL: %w", err)
	}

	if cfg.GitHub.UserAgent == "" {
		return Config{}, fmt.Errorf("GITHUB_USER_AGENT must not be empty")
	}

	if cfg.GitHub.HTTPTimeout <= 0 {
		return Config{}, fmt.Errorf("GITHUB_HTTP_TIMEOUT must be greater than 0")
	}

	if cfg.GitHub.MaxRetries < 0 {
		return Config{}, fmt.Errorf("GITHUB_MAX_RETRIES must be greater than or equal to 0")
	}

	if cfg.GitHub.InitialBackoff <= 0 {
		return Config{}, fmt.Errorf("GITHUB_INITIAL_BACKOFF must be greater than 0")
	}

	if cfg.GitHub.MaxBackoff <= 0 {
		return Config{}, fmt.Errorf("GITHUB_MAX_BACKOFF must be greater than 0")
	}

	if cfg.GitHub.InitialBackoff > cfg.GitHub.MaxBackoff {
		return Config{}, fmt.Errorf("GITHUB_INITIAL_BACKOFF must be less than or equal to GITHUB_MAX_BACKOFF")
	}

	if cfg.GitHub.App.AppID < 0 {
		return Config{}, fmt.Errorf("GITHUB_APP_ID must be greater than or equal to 0")
	}

	if cfg.GitHub.App.InstallURL != "" {
		if _, err := url.ParseRequestURI(cfg.GitHub.App.InstallURL); err != nil {
			return Config{}, fmt.Errorf("GITHUB_APP_INSTALL_URL must be a valid URL: %w", err)
		}
	}

	if cfg.Sync.WorkerPollInterval <= 0 {
		return Config{}, fmt.Errorf("SYNC_WORKER_POLL_INTERVAL must be greater than 0")
	}
	if cfg.Sync.WorkerBatchSize <= 0 {
		return Config{}, fmt.Errorf("SYNC_WORKER_BATCH_SIZE must be greater than 0")
	}
	if cfg.Sync.WorkerConcurrency <= 0 {
		return Config{}, fmt.Errorf("SYNC_WORKER_CONCURRENCY must be greater than 0")
	}
	if cfg.Sync.JobTimeout <= 0 {
		return Config{}, fmt.Errorf("SYNC_JOB_TIMEOUT must be greater than 0")
	}

	if cfg.Sync.WebhookRetryInterval <= 0 {
		return Config{}, fmt.Errorf("WEBHOOK_RETRY_INTERVAL must be greater than 0")
	}
	if cfg.Sync.WebhookRetryBatchSize <= 0 {
		return Config{}, fmt.Errorf("WEBHOOK_RETRY_BATCH_SIZE must be greater than 0")
	}
	if cfg.Sync.WebhookRetryConcurrency <= 0 {
		return Config{}, fmt.Errorf("WEBHOOK_RETRY_CONCURRENCY must be greater than 0")
	}
	if cfg.Sync.WebhookRetryTimeout <= 0 {
		return Config{}, fmt.Errorf("WEBHOOK_RETRY_TIMEOUT must be greater than 0")
	}
	if cfg.Sync.GitHubRateLimitRemaining < 0 {
		return Config{}, fmt.Errorf("GITHUB_RATE_LIMIT_MIN_REMAINING must be greater than or equal to 0")
	}

	if cfg.ClickHouse.Timeout <= 0 {
		return Config{}, fmt.Errorf("CLICKHOUSE_HTTP_TIMEOUT must be greater than 0")
	}
	if cfg.Metrics.DefaultDayType != "calendar" && cfg.Metrics.DefaultDayType != "business" {
		return Config{}, fmt.Errorf("METRICS_DEFAULT_DAY_TYPE must be one of calendar, business")
	}
	if cfg.Metrics.HotspotCommitWeight < 0 {
		return Config{}, fmt.Errorf("METRICS_HOTSPOT_WEIGHT_COMMITS must be greater than or equal to 0")
	}
	if cfg.Metrics.HotspotAdditionsWeight < 0 {
		return Config{}, fmt.Errorf("METRICS_HOTSPOT_WEIGHT_ADDITIONS must be greater than or equal to 0")
	}
	if cfg.Metrics.HotspotDeletionsWeight < 0 {
		return Config{}, fmt.Errorf("METRICS_HOTSPOT_WEIGHT_DELETIONS must be greater than or equal to 0")
	}
	if cfg.Metrics.HotspotCommitWeight == 0 && cfg.Metrics.HotspotAdditionsWeight == 0 && cfg.Metrics.HotspotDeletionsWeight == 0 {
		return Config{}, fmt.Errorf("at least one hotspot metric weight must be greater than 0")
	}

	if _, err := url.ParseRequestURI(cfg.ClickHouse.DSN); err != nil {
		return Config{}, fmt.Errorf("CLICKHOUSE_DSN must be a valid URL: %w", err)
	}

	return cfg, nil
}

func (c PostgresConfig) ConnectionString() string {
	if strings.TrimSpace(c.DSN) != "" {
		return c.DSN
	}

	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%s", c.Host, c.Port),
		Path:   c.Database,
	}

	query := dsn.Query()
	query.Set("sslmode", c.SSLMode)
	dsn.RawQuery = query.Encode()

	return dsn.String()
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}

	return value, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func getInt32(key string, fallback int32) (int32, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}

	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
	}

	return int32(value), nil
}

func getInt(key string, fallback int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
	}

	return value, nil
}

func getFloat64(key string, fallback float64) (float64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float for %s: %w", key, err)
	}

	return value, nil
}

func getInt64Value(key string) int64 {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return 0
	}

	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return -1
	}

	return value
}

func getBool(key string, fallback bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}

	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}

	return value
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL: %s", value)
	}
}
