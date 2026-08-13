package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoStoreSetsCacheHeaders(t *testing.T) {
	t.Parallel()

	next := NoStore()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, req)

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
