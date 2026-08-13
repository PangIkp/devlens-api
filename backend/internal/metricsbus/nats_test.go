package metricsbus

import (
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/nats-io/nats.go"
)

func TestCalculationRequestForEventUsesEventRange(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	to := time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)

	req := calculationRequestForEvent(syncjob.SyncCompletedEvent{
		RepositoryID: "repo-1",
		SyncJobID:    "job-1",
		OccurredAt:   to,
		From:         from,
		To:           to,
	})

	if !req.From.Equal(from) {
		t.Fatalf("expected from %s, got %s", from, req.From)
	}
	if !req.To.Equal(to) {
		t.Fatalf("expected to %s, got %s", to, req.To)
	}
	if req.MetricVersion != metrics.CurrentMetricVersion {
		t.Fatalf("expected metric version %q, got %q", metrics.CurrentMetricVersion, req.MetricVersion)
	}
}

func TestCalculationRequestForEventFallsBackToFullHistory(t *testing.T) {
	t.Parallel()

	occurredAt := time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)
	req := calculationRequestForEvent(syncjob.SyncCompletedEvent{
		RepositoryID: "repo-1",
		SyncJobID:    "job-1",
		OccurredAt:   occurredAt,
	})

	expectedFrom := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if !req.From.Equal(expectedFrom) {
		t.Fatalf("expected fallback from %s, got %s", expectedFrom, req.From)
	}
	if !req.To.Equal(occurredAt) {
		t.Fatalf("expected to %s, got %s", occurredAt, req.To)
	}
}

func TestMetricsStreamMatchesIgnoresSubjectOrder(t *testing.T) {
	t.Parallel()

	current := nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{dlqSubject, eventSubject, workSubject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	}
	desired := nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{eventSubject, workSubject, dlqSubject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	}

	if !metricsStreamMatches(current, desired) {
		t.Fatal("expected stream configs with reordered subjects to match")
	}
}

func TestMetricsStreamMatchesDetectsSubjectDrift(t *testing.T) {
	t.Parallel()

	current := nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{eventSubject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	}
	desired := nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{eventSubject, workSubject, dlqSubject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	}

	if metricsStreamMatches(current, desired) {
		t.Fatal("expected stream configs with missing subjects not to match")
	}
}
