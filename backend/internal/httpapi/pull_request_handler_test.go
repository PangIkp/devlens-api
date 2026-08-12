package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/pullrequest"
	"github.com/go-chi/chi/v5"
)

type stubPullRequestService struct {
	getFn func(context.Context, string) (pullrequest.Response, error)
}

func (s stubPullRequestService) GetByID(ctx context.Context, id string) (pullrequest.Response, error) {
	return s.getFn(ctx, id)
}

func TestPullRequestHandlerGet(t *testing.T) {
	t.Parallel()

	handler := NewPullRequestHandler(stubPullRequestService{
		getFn: func(_ context.Context, id string) (pullrequest.Response, error) {
			if id != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected pull request id %s", id)
			}
			return pullrequest.Response{
				ID: id,
				Repository: pullrequest.RepositoryRef{
					ID:       "bd546e60-e65d-b1fd-3713-6f56aa60f149",
					FullName: "acme/api",
				},
				Number:      12,
				Title:       "Improve sync reliability",
				Author:      "itsara",
				State:       "merged",
				CreatedAt:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				Reviews:     []pullrequest.Review{},
				FileChanges: []pullrequest.FileChange{},
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/pull-requests/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
