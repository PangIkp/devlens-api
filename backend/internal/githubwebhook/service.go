package githubwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type store interface {
	FindRepositoryByGithubID(context.Context, int64) (*repositoryMatch, error)
	EnqueueWebhookSync(context.Context, *string, *int64, string, string, *string, []byte, bool) (enqueueResult, error)
}

type installationEventHandler interface {
	HandleInstallationEvent(context.Context, string, int64, string) error
}

type Service struct {
	store         store
	webhookSecret string
	installations installationEventHandler
	now           func() time.Time
}

func NewService(store store, webhookSecret string, installations installationEventHandler) *Service {
	return &Service{
		store:         store,
		webhookSecret: webhookSecret,
		installations: installations,
		now:           time.Now,
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
		if match != nil {
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

	if !result.duplicate && isInstallationEvent(req.EventType) && installationID != nil && s.installations != nil {
		if err := s.installations.HandleInstallationEvent(ctx, req.EventType, *installationID, payload.Action); err != nil {
			return HandleResult{}, err
		}
	}

	return HandleResult{
		DeliveryID: result.deliveryID,
		EventType:  req.EventType,
		Duplicate:  result.duplicate,
		Enqueued:   enqueueJob && !result.duplicate,
		SyncJobID:  result.syncJobID,
		ReceivedAt: result.receivedAt,
		Action:     action,
	}, nil
}

func parsePayload(body []byte) (payloadEnvelope, error) {
	if len(body) == 0 {
		return payloadEnvelope{}, fmt.Errorf("decode webhook payload: empty body")
	}

	var payload payloadEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return payloadEnvelope{}, fmt.Errorf("decode webhook payload: %w", err)
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
	case "pull_request", "pull_request_review", "push":
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
