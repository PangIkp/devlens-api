package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTraceIDUsesRequestHeader(t *testing.T) {
	t.Parallel()

	next := TraceID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := TraceIDFromContext(r.Context()); got != "trace-123" {
			t.Fatalf("unexpected trace id %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, "trace-123")
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get(TraceIDHeader); got != "trace-123" {
		t.Fatalf("unexpected response trace id %q", got)
	}
}

func TestTraceIDGeneratesWhenMissing(t *testing.T) {
	t.Parallel()

	next := TraceID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := TraceIDFromContext(r.Context()); got == "" {
			t.Fatal("expected trace id in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get(TraceIDHeader); got == "" {
		t.Fatal("expected response trace id")
	}
}
