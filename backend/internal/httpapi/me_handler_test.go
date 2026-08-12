package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

	req := httptest.NewRequest(http.MethodGet, "/me?userId=8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
