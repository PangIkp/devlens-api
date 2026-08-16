package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PangIkp/devlens/backend/internal/auth"
)

type stubAuthenticator struct {
	principal auth.SessionPrincipal
	err       error
}

func (s stubAuthenticator) Authenticate(context.Context, string) (auth.SessionPrincipal, error) {
	return s.principal, s.err
}

func TestOptionalAuthInjectsPrincipal(t *testing.T) {
	t.Parallel()

	mw := OptionalAuth(stubAuthenticator{
		principal: auth.SessionPrincipal{
			SessionID: "session-123",
			User: auth.User{
				ID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
			},
		},
	})

	next := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("expected principal in context")
		}
		if principal.SessionID != "session-123" {
			t.Fatalf("unexpected session id %q", principal.SessionID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestRequireAuthRejectsMissingPrincipal(t *testing.T) {
	t.Parallel()

	next := RequireAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
