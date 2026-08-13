package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/auth"
	"github.com/PangIkp/devlens/backend/internal/userprofile"
	"github.com/go-chi/chi/v5"
)

type stubMeService struct {
	getFn func(context.Context, string) (userprofile.Response, error)
}

func (s stubMeService) Get(ctx context.Context, userID string) (userprofile.Response, error) {
	return s.getFn(ctx, userID)
}

func TestMeHandlerGet(t *testing.T) {
	t.Parallel()

	handler := NewMeHandler(stubMeService{
		getFn: func(_ context.Context, userID string) (userprofile.Response, error) {
			if userID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected user id %s", userID)
			}
			return userprofile.Response{
				ID:        userID,
				Email:     "devlens@example.com",
				CreatedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.SessionPrincipal{
		SessionID: "5b92a80c-ef85-4f3f-90bd-d506e9ab7a9d",
		User: auth.User{
			ID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
		},
	}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("expected ETag header")
	}
}

func TestMeHandlerGetUnauthorizedWithoutPrincipal(t *testing.T) {
	t.Parallel()

	handler := NewMeHandler(stubMeService{
		getFn: func(_ context.Context, _ string) (userprofile.Response, error) {
			t.Fatal("service should not be called")
			return userprofile.Response{}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Code != ErrorCodeUnauthorized {
		t.Fatalf("expected %s, got %s", ErrorCodeUnauthorized, body.Error.Code)
	}
}

func TestMeHandlerGetReturnsNotModifiedWhenETagMatches(t *testing.T) {
	t.Parallel()

	handler := NewMeHandler(stubMeService{
		getFn: func(_ context.Context, userID string) (userprofile.Response, error) {
			return userprofile.Response{
				ID:        userID,
				Email:     "devlens@example.com",
				CreatedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	ctx := auth.WithPrincipal(context.Background(), auth.SessionPrincipal{
		SessionID: "5b92a80c-ef85-4f3f-90bd-d506e9ab7a9d",
		User: auth.User{
			ID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
		},
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)

	req := httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	req.Header.Set("If-None-Match", firstRec.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec.Code)
	}
}
