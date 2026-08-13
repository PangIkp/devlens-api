package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PangIkp/devlens/backend/internal/githubwebhook"
	"github.com/go-chi/chi/v5"
)

type stubGitHubWebhookService struct {
	handleFn func(context.Context, githubwebhook.HandleRequest) (githubwebhook.HandleResult, error)
	retryFn  func(context.Context, string) (githubwebhook.HandleResult, error)
}

func (s stubGitHubWebhookService) Handle(ctx context.Context, req githubwebhook.HandleRequest) (githubwebhook.HandleResult, error) {
	return s.handleFn(ctx, req)
}

func (s stubGitHubWebhookService) Retry(ctx context.Context, deliveryID string) (githubwebhook.HandleResult, error) {
	return s.retryFn(ctx, deliveryID)
}

func TestGitHubWebhookHandlerInvalidPayload(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewGitHubWebhookHandler(stubGitHubWebhookService{
		handleFn: func(context.Context, githubwebhook.HandleRequest) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, githubwebhook.ErrInvalidPayload
		},
		retryFn: func(context.Context, string) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected validation error, got %q", body.Error.Code)
	}
}

func TestGitHubWebhookHandlerRetryNotAllowed(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewGitHubWebhookHandler(stubGitHubWebhookService{
		handleFn: func(context.Context, githubwebhook.HandleRequest) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, nil
		},
		retryFn: func(context.Context, string) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, githubwebhook.ErrRetryNotAllowed
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook-deliveries/delivery-1/retry", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestGitHubWebhookHandlerUnexpectedError(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewGitHubWebhookHandler(stubGitHubWebhookService{
		handleFn: func(context.Context, githubwebhook.HandleRequest) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, errors.New("boom")
		},
		retryFn: func(context.Context, string) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
