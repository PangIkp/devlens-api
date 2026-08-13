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

	"github.com/PangIkp/devlens/backend/internal/auth"
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

	if body.Dependencies.ClickHouse.Status != "unavailable" {
		t.Fatalf("expected unavailable clickhouse status, got %q", body.Dependencies.ClickHouse.Status)
	}
}

func TestReadinessHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:   stubHealthChecker{},
		ClickHouse: stubHealthChecker{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/readiness", nil)
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

func TestReadinessHandlerUnavailableClickHouse(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:   stubHealthChecker{},
		ClickHouse: stubHealthChecker{err: errors.New("clickhouse unavailable")},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/readiness", nil)
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

func TestCORSPreflightAllowedOrigin(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:       stubHealthChecker{},
		AllowedOrigins: []string{"http://localhost:5173"},
	})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("unexpected allow origin %q", got)
	}
}

func TestCORSOmitsHeadersForDisallowedOrigin(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:       stubHealthChecker{},
		AllowedOrigins: []string{"http://localhost:5173"},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected empty allow origin, got %q", got)
	}
}

func TestRouterSetsTraceIDHeader(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres: stubHealthChecker{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Trace-Id", "trace-abc")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Trace-Id"); got != "trace-abc" {
		t.Fatalf("unexpected trace id %q", got)
	}
}

func TestRouterSetsNoStoreCacheHeaders(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres: stubHealthChecker{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unexpected cache control %q", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("unexpected pragma %q", got)
	}
	if got := rec.Header().Get("Expires"); got != "0" {
		t.Fatalf("unexpected expires %q", got)
	}
}

func TestRateLimitReturnsTooManyRequests(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:          stubHealthChecker{},
		RateLimitRequests: 1,
		RateLimitWindow:   time.Minute,
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req1.RemoteAddr = "127.0.0.1:5000"
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	req2.RemoteAddr = "127.0.0.1:5000"
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	req3.RemoteAddr = "127.0.0.1:5000"
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec3.Code)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec3.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Code != ErrorCodeTooManyRequests {
		t.Fatalf("expected %s, got %s", ErrorCodeTooManyRequests, body.Error.Code)
	}
}

func TestRateLimitUsesAuthenticatedUserKey(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:          stubHealthChecker{},
		RateLimitRequests: 1,
		RateLimitWindow:   time.Minute,
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	req1.RemoteAddr = "127.0.0.1:5000"
	req1 = req1.WithContext(auth.WithPrincipal(req1.Context(), auth.SessionPrincipal{User: auth.User{ID: "user-1"}}))
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	req2.RemoteAddr = "127.0.0.1:5000"
	req2 = req2.WithContext(auth.WithPrincipal(req2.Context(), auth.SessionPrincipal{User: auth.User{ID: "user-2"}}))
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec2.Code)
	}
}
