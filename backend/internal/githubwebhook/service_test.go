package githubwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

type stubStore struct {
	findRepositoryByGithubIDFn func(context.Context, int64) (*repositoryMatch, error)
	enqueueWebhookSyncFn       func(context.Context, *string, string, string, *string, []byte, bool) (enqueueResult, error)
}

func (s stubStore) FindRepositoryByGithubID(ctx context.Context, githubID int64) (*repositoryMatch, error) {
	return s.findRepositoryByGithubIDFn(ctx, githubID)
}

func (s stubStore) EnqueueWebhookSync(ctx context.Context, repositoryID *string, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
	return s.enqueueWebhookSyncFn(ctx, repositoryID, deliveryID, eventType, action, payload, enqueueJob)
}

func TestHandleEnqueuesSupportedEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(_ context.Context, githubID int64) (*repositoryMatch, error) {
			if githubID != 42 {
				t.Fatalf("unexpected github id %d", githubID)
			}
			return &repositoryMatch{ID: "repo-1"}, nil
		},
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			if repositoryID == nil || *repositoryID != "repo-1" {
				t.Fatalf("unexpected repository id %v", repositoryID)
			}
			if deliveryID != "delivery-1" || eventType != "pull_request" || action == nil || *action != "opened" || !enqueueJob {
				t.Fatalf("unexpected enqueue args")
			}
			return enqueueResult{deliveryID: deliveryID, syncJobID: stringPtr("job-1"), receivedAt: now}, nil
		},
	}, "top-secret")

	body := []byte(`{"action":"opened","repository":{"id":42,"full_name":"devlens-labs/devlens-api"}}`)
	result, err := service.Handle(context.Background(), HandleRequest{
		DeliveryID: "delivery-1",
		EventType:  "pull_request",
		Signature:  sign("top-secret", body),
		Body:       body,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Enqueued || result.SyncJobID == nil || *result.SyncJobID != "job-1" {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestHandleTreatsDuplicateAsIdempotent(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return &repositoryMatch{ID: "repo-1"}, nil },
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			return enqueueResult{deliveryID: deliveryID, duplicate: true, receivedAt: time.Now().UTC()}, nil
		},
	}, "top-secret")

	body := []byte(`{"repository":{"id":42}}`)
	result, err := service.Handle(context.Background(), HandleRequest{
		DeliveryID: "delivery-1",
		EventType:  "push",
		Signature:  sign("top-secret", body),
		Body:       body,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Duplicate {
		t.Fatalf("expected duplicate result %+v", result)
	}
}

func TestHandleUnsupportedEventStoresWithoutEnqueue(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return &repositoryMatch{ID: "repo-1"}, nil },
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			if enqueueJob {
				t.Fatal("enqueueJob should be false")
			}
			return enqueueResult{deliveryID: deliveryID, receivedAt: time.Now().UTC()}, nil
		},
	}, "top-secret")

	body := []byte(`{"repository":{"id":42}}`)
	result, err := service.Handle(context.Background(), HandleRequest{
		DeliveryID: "delivery-2",
		EventType:  "installation",
		Signature:  sign("top-secret", body),
		Body:       body,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Enqueued {
		t.Fatalf("expected not enqueued %+v", result)
	}
}

func TestHandleRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return nil, nil },
		enqueueWebhookSyncFn: func(context.Context, *string, string, string, *string, []byte, bool) (enqueueResult, error) {
			t.Fatal("enqueue should not be called")
			return enqueueResult{}, nil
		},
	}, "top-secret")

	_, err := service.Handle(context.Background(), HandleRequest{
		DeliveryID: "delivery-3",
		EventType:  "push",
		Signature:  "sha256=deadbeef",
		Body:       []byte(`{"repository":{"id":42}}`),
	})
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected invalid signature, got %v", err)
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func stringPtr(value string) *string {
	return &value
}
