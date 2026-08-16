package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/auth"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/githubwebhook"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/go-chi/chi/v5"
)

type stubGitHubWebhookService struct {
	handleFn func(context.Context, githubwebhook.HandleRequest) (githubwebhook.HandleResult, error)
	retryFn  func(context.Context, string) (githubwebhook.HandleResult, error)
}

type stubWebhookAuthenticator struct{}

func (stubWebhookAuthenticator) Authenticate(context.Context, string) (auth.SessionPrincipal, error) {
	return auth.SessionPrincipal{
		SessionID: "session-1",
		User: auth.User{
			ID:        "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
			Email:     "owner@example.com",
			CreatedAt: time.Now().UTC(),
		},
	}, nil
}

type stubWebhookAuthorizer struct {
	authorizeWebhookDeliveryFn func(context.Context, string, string, ...string) error
}

func (s stubWebhookAuthorizer) AuthorizeOrganization(context.Context, string, string, ...string) error {
	return nil
}

func (s stubWebhookAuthorizer) AuthorizeRepository(context.Context, string, string, ...string) error {
	return nil
}

func (s stubWebhookAuthorizer) AuthorizePullRequest(context.Context, string, string, ...string) error {
	return nil
}

func (s stubWebhookAuthorizer) AuthorizeSyncJob(context.Context, string, string, ...string) error {
	return nil
}

func (s stubWebhookAuthorizer) AuthorizeWebhookDelivery(ctx context.Context, userID string, deliveryID string, roles ...string) error {
	if s.authorizeWebhookDeliveryFn == nil {
		return nil
	}
	return s.authorizeWebhookDeliveryFn(ctx, userID, deliveryID, roles...)
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

func TestGitHubWebhookRetryRequiresAuthenticationWhenAuthorizerConfigured(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Use(middleware.OptionalAuth(stubWebhookAuthenticator{}))
	NewGitHubWebhookHandler(stubGitHubWebhookService{
		handleFn: func(context.Context, githubwebhook.HandleRequest) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, nil
		},
		retryFn: func(context.Context, string) (githubwebhook.HandleResult, error) {
			t.Fatal("retry should not be called without auth")
			return githubwebhook.HandleResult{}, nil
		},
	}, stubWebhookAuthorizer{
		authorizeWebhookDeliveryFn: func(context.Context, string, string, ...string) error {
			t.Fatal("authorize should not be called without auth")
			return nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook-deliveries/delivery-1/retry", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestGitHubWebhookRetryUsesAuthorizationWhenConfigured(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	router.Use(middleware.OptionalAuth(stubWebhookAuthenticator{}))
	NewGitHubWebhookHandler(stubGitHubWebhookService{
		handleFn: func(context.Context, githubwebhook.HandleRequest) (githubwebhook.HandleResult, error) {
			return githubwebhook.HandleResult{}, nil
		},
		retryFn: func(_ context.Context, deliveryID string) (githubwebhook.HandleResult, error) {
			if deliveryID != "11111111-1111-1111-1111-111111111111" {
				t.Fatalf("unexpected delivery id %q", deliveryID)
			}
			return githubwebhook.HandleResult{DeliveryID: deliveryID, EventType: "push"}, nil
		},
	}, stubWebhookAuthorizer{
		authorizeWebhookDeliveryFn: func(_ context.Context, userID string, deliveryID string, roles ...string) error {
			if userID == "" || deliveryID != "11111111-1111-1111-1111-111111111111" {
				t.Fatalf("unexpected auth input user=%q delivery=%q", userID, deliveryID)
			}
			if len(roles) != 2 || roles[0] != authorization.RoleAdmin || roles[1] != authorization.RoleOwner {
				t.Fatalf("unexpected roles %#v", roles)
			}
			return nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/github/webhook-deliveries/11111111-1111-1111-1111-111111111111/retry", nil)
	req.Header.Set("Authorization", "Bearer token-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}
