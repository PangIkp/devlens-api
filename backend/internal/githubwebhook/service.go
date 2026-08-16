package githubwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/PangIkp/devlens/backend/internal/observability"
)

const maxWebhookRetries = 5

type store interface {
	FindRepositoryByGithubID(context.Context, int64) (*repositoryMatch, error)
	EnqueueWebhookSync(context.Context, *string, *int64, string, string, *string, []byte, bool) (enqueueResult, error)
	ProjectRepositoryEvent(context.Context, string, string, payloadEnvelope) error
	MarkDeliveryStatus(context.Context, string, string, *string, *time.Time) error
	GetStoredDelivery(context.Context, string) (*StoredDelivery, error)
	ScheduleRetry(context.Context, string, string, int, time.Time, time.Time) error
	ListRetryableDeliveryIDs(context.Context, int, time.Time) ([]string, error)
}

type installationEventHandler interface {
	HandleInstallationEvent(context.Context, string, int64, string) error
}

type Service struct {
	store            store
	webhookSecret    string
	installations    installationEventHandler
	now              func() time.Time
	metrics          *observability.Metrics
	retryConcurrency int
	retryTimeout     time.Duration
}

func NewService(store store, webhookSecret string, installations installationEventHandler, metrics *observability.Metrics) *Service {
	return &Service{
		store:            store,
		webhookSecret:    webhookSecret,
		installations:    installations,
		now:              time.Now,
		metrics:          metrics,
		retryConcurrency: 1,
		retryTimeout:     30 * time.Second,
	}
}

func (s *Service) ConfigureRetryProcessing(concurrency int, timeout time.Duration) {
	if concurrency > 0 {
		s.retryConcurrency = concurrency
	}
	if timeout > 0 {
		s.retryTimeout = timeout
	}
}

func (s *Service) Handle(ctx context.Context, req HandleRequest) (HandleResult, error) {
	if strings.TrimSpace(req.DeliveryID) == "" {
		return HandleResult{}, ErrMissingDelivery
	}
	if strings.TrimSpace(req.EventType) == "" {
		return HandleResult{}, ErrMissingEvent
	}
	if err := verifySignature(s.webhookSecret, req.Body, req.Signature); err != nil {
		return HandleResult{}, err
	}

	payload, err := parsePayload(req.Body)
	if err != nil {
		return HandleResult{}, err
	}

	var repositoryID *string
	if payload.Repository.ID > 0 {
		match, err := s.store.FindRepositoryByGithubID(ctx, payload.Repository.ID)
		if err != nil {
			return HandleResult{}, err
		}
		// A deactivated repository keeps its webhook delivery recorded (for
		// audit/dedup) but must not have new data projected or a sync job
		// enqueued for it — leaving repositoryID nil short-circuits both.
		if match != nil && !match.Inactive {
			repositoryID = &match.ID
		}
	}

	enqueueJob := isSupportedEvent(req.EventType) && repositoryID != nil
	action := optionalString(payload.Action)
	installationID := optionalInt64(payload.Installation.ID)
	result, err := s.store.EnqueueWebhookSync(ctx, repositoryID, installationID, req.DeliveryID, req.EventType, action, req.Body, enqueueJob)
	if err != nil {
		return HandleResult{}, err
	}

	if !result.duplicate {
		if repositoryID != nil && isSupportedEvent(req.EventType) {
			if err := s.store.ProjectRepositoryEvent(ctx, *repositoryID, req.EventType, payload); err != nil {
				_ = s.scheduleRetryFailure(ctx, req.DeliveryID, 0, err)
				s.recordWebhookDelay(req.EventType, "failed", result.receivedAt)
				return HandleResult{}, err
			}
		}
		if err := s.processPersistedDelivery(ctx, req.DeliveryID, req.EventType, payload.Action, installationID); err != nil {
			_ = s.scheduleRetryFailure(ctx, req.DeliveryID, 0, err)
			s.recordWebhookDelay(req.EventType, "failed", result.receivedAt)
			return HandleResult{}, err
		}
	}

	s.recordWebhookDelay(req.EventType, result.processingStatus, result.receivedAt)

	return HandleResult{
		DeliveryID:       result.deliveryID,
		EventType:        req.EventType,
		Duplicate:        result.duplicate,
		Enqueued:         enqueueJob && !result.duplicate,
		ProcessingStatus: result.processingStatus,
		SyncJobID:        result.syncJobID,
		ReceivedAt:       result.receivedAt,
		Action:           action,
	}, nil
}

func (s *Service) Retry(ctx context.Context, deliveryID string) (HandleResult, error) {
	stored, err := s.store.GetStoredDelivery(ctx, strings.TrimSpace(deliveryID))
	if err != nil {
		return HandleResult{}, err
	}
	if stored == nil {
		return HandleResult{}, ErrDeliveryNotFound
	}
	if stored.ProcessingStatus != "failed" {
		return HandleResult{}, ErrRetryNotAllowed
	}

	if stored.RepositoryID != nil && isSupportedEvent(stored.EventType) {
		payload, err := parsePayload(stored.Payload)
		if err != nil {
			return HandleResult{}, err
		}
		if err := s.store.ProjectRepositoryEvent(ctx, *stored.RepositoryID, stored.EventType, payload); err != nil {
			_ = s.scheduleRetryFailure(ctx, stored.DeliveryID, stored.RetryCount, err)
			s.recordWebhookDelay(stored.EventType, "failed", stored.ReceivedAt)
			return HandleResult{}, err
		}
	}

	if err := s.processPersistedDelivery(ctx, stored.DeliveryID, stored.EventType, valueOrEmpty(stored.Action), stored.InstallationID); err != nil {
		_ = s.scheduleRetryFailure(ctx, stored.DeliveryID, stored.RetryCount, err)
		s.recordWebhookDelay(stored.EventType, "failed", stored.ReceivedAt)
		return HandleResult{}, err
	}

	processedAt := s.now().UTC()
	if err := s.store.MarkDeliveryStatus(ctx, stored.DeliveryID, deriveProcessedStatus(stored.EventType, stored.SyncJobID != nil), nil, &processedAt); err != nil {
		return HandleResult{}, err
	}
	s.recordWebhookDelay(stored.EventType, deriveProcessedStatus(stored.EventType, stored.SyncJobID != nil), stored.ReceivedAt)

	return HandleResult{
		DeliveryID:       stored.DeliveryID,
		EventType:        stored.EventType,
		Duplicate:        false,
		Enqueued:         stored.SyncJobID != nil,
		ProcessingStatus: deriveProcessedStatus(stored.EventType, stored.SyncJobID != nil),
		SyncJobID:        stored.SyncJobID,
		ReceivedAt:       stored.ReceivedAt,
		Action:           stored.Action,
	}, nil
}

func (s *Service) RetryFailedPending(ctx context.Context, limit int) error {
	ids, err := s.store.ListRetryableDeliveryIDs(ctx, limit, s.now().UTC())
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	concurrency := s.retryConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	errCh := make(chan error, len(ids))
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(deliveryID string) {
			defer wg.Done()
			defer func() { <-sem }()

			retryCtx := ctx
			if s.retryTimeout > 0 {
				var cancel context.CancelFunc
				retryCtx, cancel = context.WithTimeout(ctx, s.retryTimeout)
				defer cancel()
			}
			if _, err := s.Retry(retryCtx, deliveryID); err != nil {
				errCh <- err
			}
		}(id)
	}
	wg.Wait()
	close(errCh)
	for range errCh {
	}
	return nil
}

func (s *Service) processPersistedDelivery(ctx context.Context, deliveryID string, eventType string, action string, installationID *int64) error {
	if isInstallationEvent(eventType) && installationID != nil && s.installations != nil {
		if err := s.installations.HandleInstallationEvent(ctx, eventType, *installationID, action); err != nil {
			return err
		}
		processedAt := s.now().UTC()
		return s.store.MarkDeliveryStatus(ctx, deliveryID, "processed", nil, &processedAt)
	}
	if !isSupportedEvent(eventType) {
		processedAt := s.now().UTC()
		return s.store.MarkDeliveryStatus(ctx, deliveryID, "ignored", nil, &processedAt)
	}
	return nil
}

func parsePayload(body []byte) (payloadEnvelope, error) {
	if len(body) == 0 {
		return payloadEnvelope{}, fmt.Errorf("%w: empty body", ErrInvalidPayload)
	}

	var payload payloadEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return payloadEnvelope{}, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	return payload, nil
}

func verifySignature(secret string, body []byte, signature string) error {
	if strings.TrimSpace(secret) == "" {
		return ErrInvalidSignature
	}
	if !strings.HasPrefix(strings.TrimSpace(signature), "sha256=") {
		return ErrInvalidSignature
	}

	expectedMAC := hmac.New(sha256.New, []byte(secret))
	expectedMAC.Write(body)
	expected := "sha256=" + hex.EncodeToString(expectedMAC.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return ErrInvalidSignature
	}
	return nil
}

func isSupportedEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "pull_request", "pull_request_review", "push", "workflow_run", "deployment", "deployment_status":
		return true
	default:
		return false
	}
}

func isInstallationEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "installation", "installation_repositories":
		return true
	default:
		return false
	}
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalInt64(value int64) *int64 {
	if value < 1 {
		return nil
	}
	copy := value
	return &copy
}

func deriveProcessedStatus(eventType string, enqueued bool) string {
	if enqueued && isSupportedEvent(eventType) {
		return "enqueued"
	}
	if isInstallationEvent(eventType) {
		return "processed"
	}
	return "ignored"
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) scheduleRetryFailure(ctx context.Context, deliveryID string, retryCount int, err error) error {
	failedAt := s.now().UTC()
	attempt := retryCount + 1
	if attempt >= maxWebhookRetries {
		message := err.Error()
		return s.store.MarkDeliveryStatus(ctx, deliveryID, "dead_letter", &message, &failedAt)
	}
	nextRetryAt := failedAt.Add(retryDelayForAttempt(attempt))
	return s.store.ScheduleRetry(ctx, deliveryID, err.Error(), attempt, failedAt, nextRetryAt)
}

func retryDelayForAttempt(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 30 * time.Second
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= 15*time.Minute {
			return 15 * time.Minute
		}
	}
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

func (s *Service) recordWebhookDelay(eventType, status string, receivedAt time.Time) {
	if s == nil || s.metrics == nil || receivedAt.IsZero() {
		return
	}
	delay := s.now().UTC().Sub(receivedAt.UTC())
	if delay < 0 {
		delay = 0
	}
	s.metrics.RecordWebhookProcessingDelay(eventType, status, delay)
}
