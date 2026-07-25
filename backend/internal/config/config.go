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
	NATS       NATSConfig
}

type HTTPConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
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
}

type NATSConfig struct {
	URL string
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

	pgMaxConns, err := getInt32("POSTGRES_MAX_CONNS", 10)
	if err != nil {
		return Config{}, err
	}

	pgMinConns, err := getInt32("POSTGRES_MIN_CONNS", 0)
	if err != nil {
		return Config{}, err
	}

	httpCfg := HTTPConfig{
		Addr:            getEnv("HTTP_ADDR", ":8080"),
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		IdleTimeout:     idleTimeout,
		ShutdownTimeout: shutdownTimeout,
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
			Host:     getEnv("CLICKHOUSE_HOST", "clickhouse"),
			Port:     getEnv("CLICKHOUSE_PORT", "8123"),
			User:     getEnv("CLICKHOUSE_USER", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
			Database: getEnv("CLICKHOUSE_DATABASE", "devlens"),
			DSN:      getEnv("CLICKHOUSE_DSN", "http://clickhouse:8123"),
		},
		NATS: NATSConfig{
			URL: getEnv("NATS_URL", "nats://nats:4222"),
		},
	}

	if strings.TrimSpace(cfg.HTTP.Addr) == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR must not be empty")
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
