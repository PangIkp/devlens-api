package metricsbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
)

type stubFallbackCalculator struct {
	calls chan syncjob.SyncCompletedEvent
	err   error
}

func (s *stubFallbackCalculator) CalculateRepositoryMetrics(_ context.Context, repositoryID string, req metrics.CalculationRequest) error {
	if s.calls != nil {
		s.calls <- syncjob.SyncCompletedEvent{
			RepositoryID: repositoryID,
			From:         req.From,
			To:           req.To,
		}
	}
	return s.err
}

func TestPublisherFallsBackWhenPublishFailsIntegration(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	client, err := Open(cfg.NATS.URL, nil)
	if err != nil {
		t.Skipf("skip nats integration test: nats unavailable: %v", err)
	}
	client.Close()

	calls := make(chan syncjob.SyncCompletedEvent, 1)
	publisher := NewPublisher(nil, client, &stubFallbackCalculator{calls: calls})
	event := syncjob.SyncCompletedEvent{
		RepositoryID: "repo-fallback",
		SyncJobID:    "job-fallback",
		OccurredAt:   time.Now().UTC(),
	}

	if err := publisher.PublishRepositorySyncCompleted(context.Background(), event); err != nil {
		t.Fatalf("expected fallback publish to succeed, got %v", err)
	}

	select {
	case call := <-calls:
		if call.RepositoryID != event.RepositoryID {
			t.Fatalf("expected fallback repository %q, got %q", event.RepositoryID, call.RepositoryID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected fallback calculator to be invoked")
	}
}

func TestPublisherReturnsCombinedErrorWhenFallbackAlsoFailsIntegration(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	client, err := Open(cfg.NATS.URL, nil)
	if err != nil {
		t.Skipf("skip nats integration test: nats unavailable: %v", err)
	}
	client.Close()

	publisher := NewPublisher(nil, client, &stubFallbackCalculator{err: errors.New("fallback failed")})
	err = publisher.PublishRepositorySyncCompleted(context.Background(), syncjob.SyncCompletedEvent{
		RepositoryID: "repo-fallback-error",
		SyncJobID:    "job-fallback-error",
		OccurredAt:   time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected combined publish/fallback error")
	}
}

func TestConsumerProcessesPublishedEventIntegration(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	client, err := Open(cfg.NATS.URL, nil)
	if err != nil {
		t.Skipf("skip nats integration test: nats unavailable: %v", err)
	}
	t.Cleanup(client.Close)

	calls := make(chan syncjob.SyncCompletedEvent, 1)
	consumer := NewConsumer(nil, client, &stubFallbackCalculator{calls: calls})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	event := syncjob.SyncCompletedEvent{
		RepositoryID: "repo-consumer",
		SyncJobID:    "job-consumer-" + time.Now().UTC().Format("150405.000000000"),
		OccurredAt:   time.Now().UTC(),
		From:         time.Now().UTC().Add(-24 * time.Hour),
		To:           time.Now().UTC(),
	}
	if err := client.PublishRepositorySyncCompleted(context.Background(), event); err != nil {
		t.Fatalf("publish metrics event: %v", err)
	}

	select {
	case call := <-calls:
		if call.RepositoryID != event.RepositoryID {
			t.Fatalf("expected repository id %q, got %q", event.RepositoryID, call.RepositoryID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected consumer to process published event")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("consumer shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected consumer shutdown")
	}
}
