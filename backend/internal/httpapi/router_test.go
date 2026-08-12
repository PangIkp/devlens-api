package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
)

type stubHealthChecker struct {
	err error
}

func (s stubHealthChecker) Check(_ context.Context) error {
	return s.err
}

func TestHealthHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres: stubHealthChecker{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}

	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}

	if body.Timestamp.IsZero() {
		t.Fatal("expected timestamp to be set")
	}

	if body.Dependencies.Postgres.Status != "ok" {
		t.Fatalf("expected postgres status ok, got %q", body.Dependencies.Postgres.Status)
	}

	if time.Since(body.Timestamp) > time.Minute {
		t.Fatalf("expected recent timestamp, got %s", body.Timestamp)
	}
}

func TestHealthHandlerUnavailablePostgres(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres: stubHealthChecker{err: errors.New("database unavailable")},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}

	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Status != "degraded" {
		t.Fatalf("expected degraded status, got %q", body.Status)
	}

	if body.Dependencies.Postgres.Status != "unavailable" {
		t.Fatalf("expected unavailable postgres status, got %q", body.Dependencies.Postgres.Status)
	}
}

func TestHealthHandlerDegradedClickHouse(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:   stubHealthChecker{},
		ClickHouse: stubHealthChecker{err: errors.New("clickhouse unavailable")},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Status != "degraded" {
		t.Fatalf("expected degraded status, got %q", body.Status)
	}

	if body.Dependencies.ClickHouse.Status != "unavailable" {
		t.Fatalf("expected unavailable clickhouse status, got %q", body.Dependencies.ClickHouse.Status)
	}
}

func TestHealthHandlerIgnoresTypedNilClickHouse(t *testing.T) {
	t.Parallel()

	var clickhouseDB *clickhouse.DB

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:   stubHealthChecker{},
		ClickHouse: clickhouseDB,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
}

func TestHealthHandlerMethodNotAllowed(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres: stubHealthChecker{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("expected METHOD_NOT_ALLOWED, got %q", body.Error.Code)
	}
}

func TestNotFoundReturnsJSONError(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres: stubHealthChecker{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Code != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND, got %q", body.Error.Code)
	}
}
