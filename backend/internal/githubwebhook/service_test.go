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
	enqueueWebhookSyncFn       func(context.Context, *string, *int64, string, string, *string, []byte, bool) (enqueueResult, error)
	markStatusFn               func(context.Context, string, string, *string, *time.Time) error
	getStoredDeliveryFn        func(context.Context, string) (*StoredDelivery, error)
	scheduleRetryFn            func(context.Context, string, string, int, time.Time, time.Time) error
	listRetryableFn            func(context.Context, int, time.Time) ([]string, error)
}

func (s stubStore) FindRepositoryByGithubID(ctx context.Context, githubID int64) (*repositoryMatch, error) {
	return s.findRepositoryByGithubIDFn(ctx, githubID)
}

func (s stubStore) EnqueueWebhookSync(ctx context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
	return s.enqueueWebhookSyncFn(ctx, repositoryID, installationID, deliveryID, eventType, action, payload, enqueueJob)
}

func (s stubStore) MarkDeliveryStatus(ctx context.Context, deliveryID string, status string, message *string, processedAt *time.Time) error {
	if s.markStatusFn == nil {
		return nil
	}
	return s.markStatusFn(ctx, deliveryID, status, message, processedAt)
}

func (s stubStore) GetStoredDelivery(ctx context.Context, deliveryID string) (*StoredDelivery, error) {
	if s.getStoredDeliveryFn == nil {
		return nil, nil
	}
	return s.getStoredDeliveryFn(ctx, deliveryID)
}

func (s stubStore) ScheduleRetry(ctx context.Context, deliveryID string, message string, retryCount int, failedAt time.Time, nextRetryAt time.Time) error {
	if s.scheduleRetryFn == nil {
		return nil
	}
	return s.scheduleRetryFn(ctx, deliveryID, message, retryCount, failedAt, nextRetryAt)
}

func (s stubStore) ListRetryableDeliveryIDs(ctx context.Context, limit int, now time.Time) ([]string, error) {
	if s.listRetryableFn == nil {
		return nil, nil
	}
	return s.listRetryableFn(ctx, limit, now)
}

type stubInstallationHandler struct {
	handleFn func(context.Context, string, int64, string) error
}

func (s stubInstallationHandler) HandleInstallationEvent(ctx context.Context, eventType string, installationID int64, action string) error {
	return s.handleFn(ctx, eventType, installationID, action)
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
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			if repositoryID == nil || *repositoryID != "repo-1" {
				t.Fatalf("unexpected repository id %v", repositoryID)
			}
			if installationID != nil {
				t.Fatalf("did not expect installation id %v", installationID)
			}
			if deliveryID != "delivery-1" || eventType != "pull_request" || action == nil || *action != "opened" || !enqueueJob {
				t.Fatalf("unexpected enqueue args")
			}
			return enqueueResult{deliveryID: deliveryID, syncJobID: stringPtr("job-1"), receivedAt: now, processingStatus: "enqueued"}, nil
		},
	}, "top-secret", nil)

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
	if result.ProcessingStatus != "enqueued" {
		t.Fatalf("expected enqueued processing status, got %q", result.ProcessingStatus)
	}
}

func TestRetryReprocessesFailedInstallationDelivery(t *testing.T) {
	t.Parallel()

	var marked []string
	service := NewService(stubStore{
		getStoredDeliveryFn: func(_ context.Context, deliveryID string) (*StoredDelivery, error) {
			if deliveryID != "delivery-5" {
				t.Fatalf("unexpected delivery id %q", deliveryID)
			}
			action := "created"
			installationID := int64(77)
			return &StoredDelivery{
				DeliveryID:       deliveryID,
				EventType:        "installation",
				Action:           &action,
				InstallationID:   &installationID,
				ProcessingStatus: "failed",
				ReceivedAt:       time.Now().UTC(),
			}, nil
		},
		markStatusFn: func(_ context.Context, deliveryID string, status string, message *string, processedAt *time.Time) error {
			marked = append(marked, status)
			return nil
		},
	}, "top-secret", stubInstallationHandler{
		handleFn: func(_ context.Context, eventType string, installationID int64, action string) error {
			if eventType != "installation" || installationID != 77 || action != "created" {
				t.Fatalf("unexpected retry args")
			}
			return nil
		},
	})

	result, err := service.Retry(context.Background(), "delivery-5")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ProcessingStatus != "processed" {
		t.Fatalf("expected processed, got %q", result.ProcessingStatus)
	}
	if len(marked) == 0 {
		t.Fatal("expected delivery status to be marked")
	}
}

func TestRetryFailedPendingProcessesQueuedDeliveries(t *testing.T) {
	t.Parallel()

	called := 0
	service := NewService(stubStore{
		listRetryableFn: func(_ context.Context, limit int, _ time.Time) ([]string, error) {
			if limit != 10 {
				t.Fatalf("unexpected limit %d", limit)
			}
			return []string{"delivery-1"}, nil
		},
		getStoredDeliveryFn: func(_ context.Context, deliveryID string) (*StoredDelivery, error) {
			action := "created"
			installationID := int64(55)
			return &StoredDelivery{
				DeliveryID:       deliveryID,
				EventType:        "installation",
				Action:           &action,
				InstallationID:   &installationID,
				ProcessingStatus: "failed",
				ReceivedAt:       time.Now().UTC(),
			}, nil
		},
		markStatusFn: func(context.Context, string, string, *string, *time.Time) error { return nil },
	}, "top-secret", stubInstallationHandler{
		handleFn: func(context.Context, string, int64, string) error {
			called++
			return nil
		},
	})

	if err := service.RetryFailedPending(context.Background(), 10); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if called != 1 {
		t.Fatalf("expected one retry processing call, got %d", called)
	}
}

func TestHandleSchedulesRetryWhenInstallationProcessingFails(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	var recordedRetryCount int
	var recordedFailedAt time.Time
	var recordedNextRetryAt time.Time

	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) {
			return nil, nil
		},
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			if enqueueJob {
				t.Fatal("installation event should not enqueue a sync job")
			}
			return enqueueResult{deliveryID: deliveryID, receivedAt: now, processingStatus: "ignored"}, nil
		},
		scheduleRetryFn: func(_ context.Context, deliveryID string, message string, retryCount int, failedAt time.Time, nextRetryAt time.Time) error {
			if deliveryID != "delivery-6" {
				t.Fatalf("unexpected delivery id %q", deliveryID)
			}
			if message == "" {
				t.Fatal("expected retry message")
			}
			recordedRetryCount = retryCount
			recordedFailedAt = failedAt
			recordedNextRetryAt = nextRetryAt
			return nil
		},
	}, "top-secret", stubInstallationHandler{
		handleFn: func(context.Context, string, int64, string) error {
			return errors.New("temporary github app failure")
		},
	})
	service.now = func() time.Time { return now }

	body := []byte(`{"action":"created","installation":{"id":99}}`)
	_, err := service.Handle(context.Background(), HandleRequest{
		DeliveryID: "delivery-6",
		EventType:  "installation",
		Signature:  sign("top-secret", body),
		Body:       body,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if recordedRetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", recordedRetryCount)
	}
	if !recordedFailedAt.Equal(now) {
		t.Fatalf("unexpected failed at %s", recordedFailedAt)
	}
	if !recordedNextRetryAt.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("unexpected next retry at %s", recordedNextRetryAt)
	}
}

func TestHandleTreatsDuplicateAsIdempotent(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return &repositoryMatch{ID: "repo-1"}, nil },
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			return enqueueResult{deliveryID: deliveryID, duplicate: true, receivedAt: time.Now().UTC(), processingStatus: "processed"}, nil
		},
	}, "top-secret", nil)

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

	var handled bool
	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return &repositoryMatch{ID: "repo-1"}, nil },
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			if enqueueJob {
				t.Fatal("enqueueJob should be false")
			}
			if installationID == nil || *installationID != 99 {
				t.Fatalf("expected installation id 99, got %v", installationID)
			}
			return enqueueResult{deliveryID: deliveryID, receivedAt: time.Now().UTC(), processingStatus: "ignored"}, nil
		},
	}, "top-secret", stubInstallationHandler{
		handleFn: func(_ context.Context, eventType string, installationID int64, action string) error {
			handled = true
			if eventType != "installation" || installationID != 99 || action != "created" {
				t.Fatalf("unexpected installation event args")
			}
			return nil
		},
	})

	body := []byte(`{"action":"created","installation":{"id":99},"repository":{"id":42}}`)
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
	if !handled {
		t.Fatal("expected installation handler to be called")
	}
}

func TestHandleProcessesOutOfOrderInstallationDeletedEventIdempotently(t *testing.T) {
	t.Parallel()

	var markedStatus string
	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return nil, nil },
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			if enqueueJob {
				t.Fatal("installation deletion should not enqueue a sync job")
			}
			if installationID == nil || *installationID != 99 {
				t.Fatalf("expected installation id 99, got %v", installationID)
			}
			return enqueueResult{
				deliveryID:       deliveryID,
				receivedAt:       time.Now().UTC(),
				processingStatus: "ignored",
			}, nil
		},
		markStatusFn: func(_ context.Context, deliveryID string, status string, message *string, processedAt *time.Time) error {
			if deliveryID != "delivery-deleted-1" {
				t.Fatalf("unexpected delivery id %q", deliveryID)
			}
			if processedAt == nil || processedAt.IsZero() {
				t.Fatal("expected processedAt to be recorded")
			}
			markedStatus = status
			return nil
		},
	}, "top-secret", stubInstallationHandler{
		handleFn: func(_ context.Context, eventType string, installationID int64, action string) error {
			if eventType != "installation" || installationID != 99 || action != "deleted" {
				t.Fatalf("unexpected out-of-order installation event args")
			}
			return nil
		},
	})

	body := []byte(`{"action":"deleted","installation":{"id":99}}`)
	result, err := service.Handle(context.Background(), HandleRequest{
		DeliveryID: "delivery-deleted-1",
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
	if markedStatus != "processed" {
		t.Fatalf("expected processed status mark, got %q", markedStatus)
	}
}

func TestHandleEnqueuesWorkflowRunEvent(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return &repositoryMatch{ID: "repo-1"}, nil },
		enqueueWebhookSyncFn: func(_ context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
			if !enqueueJob || eventType != "workflow_run" {
				t.Fatalf("expected workflow_run to enqueue")
			}
			return enqueueResult{deliveryID: deliveryID, receivedAt: time.Now().UTC(), processingStatus: "enqueued"}, nil
		},
	}, "top-secret", nil)

	body := []byte(`{"action":"completed","repository":{"id":42}}`)
	result, err := service.Handle(context.Background(), HandleRequest{
		DeliveryID: "delivery-4",
		EventType:  "workflow_run",
		Signature:  sign("top-secret", body),
		Body:       body,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.Enqueued || result.ProcessingStatus != "enqueued" {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestRetrySchedulesNextAttemptWhenReprocessingFails(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)
	var gotRetryCount int
	var gotFailedAt time.Time
	var gotNextRetryAt time.Time

	service := NewService(stubStore{
		getStoredDeliveryFn: func(_ context.Context, deliveryID string) (*StoredDelivery, error) {
			action := "created"
			installationID := int64(77)
			return &StoredDelivery{
				DeliveryID:       deliveryID,
				EventType:        "installation",
				Action:           &action,
				InstallationID:   &installationID,
				ProcessingStatus: "failed",
				RetryCount:       2,
				ReceivedAt:       now.Add(-time.Minute),
			}, nil
		},
		scheduleRetryFn: func(_ context.Context, deliveryID string, message string, retryCount int, failedAt time.Time, nextRetryAt time.Time) error {
			if deliveryID != "delivery-7" {
				t.Fatalf("unexpected delivery id %q", deliveryID)
			}
			if message == "" {
				t.Fatal("expected retry message")
			}
			gotRetryCount = retryCount
			gotFailedAt = failedAt
			gotNextRetryAt = nextRetryAt
			return nil
		},
	}, "top-secret", stubInstallationHandler{
		handleFn: func(context.Context, string, int64, string) error {
			return errors.New("temporary retry failure")
		},
	})
	service.now = func() time.Time { return now }

	_, err := service.Retry(context.Background(), "delivery-7")
	if err == nil {
		t.Fatal("expected error")
	}
	if gotRetryCount != 3 {
		t.Fatalf("expected retry count 3, got %d", gotRetryCount)
	}
	if !gotFailedAt.Equal(now) {
		t.Fatalf("unexpected failed at %s", gotFailedAt)
	}
	if !gotNextRetryAt.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("unexpected next retry at %s", gotNextRetryAt)
	}
}

func TestRetryFailedPendingContinuesAfterSingleDeliveryFailure(t *testing.T) {
	t.Parallel()

	var processed []string
	service := NewService(stubStore{
		listRetryableFn: func(_ context.Context, limit int, _ time.Time) ([]string, error) {
			if limit != 10 {
				t.Fatalf("unexpected limit %d", limit)
			}
			return []string{"delivery-fail", "delivery-ok"}, nil
		},
		getStoredDeliveryFn: func(_ context.Context, deliveryID string) (*StoredDelivery, error) {
			action := "created"
			installationID := int64(55)
			return &StoredDelivery{
				DeliveryID:       deliveryID,
				EventType:        "installation",
				Action:           &action,
				InstallationID:   &installationID,
				ProcessingStatus: "failed",
				ReceivedAt:       time.Now().UTC(),
			}, nil
		},
		markStatusFn: func(_ context.Context, deliveryID string, status string, message *string, processedAt *time.Time) error {
			processed = append(processed, deliveryID+":"+status)
			return nil
		},
		scheduleRetryFn: func(_ context.Context, deliveryID string, _ string, _ int, _ time.Time, _ time.Time) error {
			processed = append(processed, deliveryID+":rescheduled")
			return nil
		},
	}, "top-secret", stubInstallationHandler{
		handleFn: func(_ context.Context, _ string, installationID int64, _ string) error {
			if installationID != 55 {
				t.Fatalf("unexpected installation id %d", installationID)
			}
			if len(processed) == 0 {
				return errors.New("first delivery fails")
			}
			return nil
		},
	})

	if err := service.RetryFailedPending(context.Background(), 10); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(processed) < 2 {
		t.Fatalf("expected both failure recovery and later processing, got %#v", processed)
	}
	if processed[0] != "delivery-fail:rescheduled" {
		t.Fatalf("unexpected first processed entry %#v", processed)
	}
	if processed[len(processed)-1] != "delivery-ok:processed" {
		t.Fatalf("expected second delivery to complete processing, got %#v", processed)
	}
}

func TestHandleRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		findRepositoryByGithubIDFn: func(context.Context, int64) (*repositoryMatch, error) { return nil, nil },
		enqueueWebhookSyncFn: func(context.Context, *string, *int64, string, string, *string, []byte, bool) (enqueueResult, error) {
			t.Fatal("enqueue should not be called")
			return enqueueResult{}, nil
		},
	}, "top-secret", nil)

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
