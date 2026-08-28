package metricsbus

import (
	"context"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
)

type stubUnitFallbackCalculator struct {
	repositoryID string
	req          metrics.CalculationRequest
	calls        int
}

func (s *stubUnitFallbackCalculator) CalculateRepositoryMetrics(_ context.Context, repositoryID string, req metrics.CalculationRequest) error {
	s.repositoryID = repositoryID
	s.req = req
	s.calls++
	return nil
}

func TestPublisherFallsBackInlineWithoutNATSClient(t *testing.T) {
	t.Parallel()

	calculator := &stubUnitFallbackCalculator{}
	publisher := NewPublisher(nil, nil, calculator)
	event := syncjob.SyncCompletedEvent{
		RepositoryID: "repo-inline",
		OccurredAt:   time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC),
	}

	if err := publisher.PublishRepositorySyncCompleted(context.Background(), event); err != nil {
		t.Fatalf("publish inline metrics event: %v", err)
	}

	if calculator.calls != 1 {
		t.Fatalf("expected fallback calculator to be called once, got %d", calculator.calls)
	}
	if calculator.repositoryID != event.RepositoryID {
		t.Fatalf("expected repository %q, got %q", event.RepositoryID, calculator.repositoryID)
	}
	if calculator.req.From.Format("2006-01-02") != historyStart {
		t.Fatalf("expected fallback from=%s, got %s", historyStart, calculator.req.From.Format("2006-01-02"))
	}
	if !calculator.req.To.Equal(event.OccurredAt) {
		t.Fatalf("expected fallback to=%s, got %s", event.OccurredAt, calculator.req.To)
	}
	if calculator.req.MetricVersion != metrics.CurrentMetricVersion {
		t.Fatalf("expected metric version %d, got %d", metrics.CurrentMetricVersion, calculator.req.MetricVersion)
	}
}
