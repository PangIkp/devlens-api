package insightbus

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/nats-io/nats.go"
)

type stubGenerator struct {
	organizationID string
	repositoryID   string
	from           time.Time
	to             time.Time
	calls          int
}

func (s *stubGenerator) RefreshRepository(_ context.Context, organizationID string, repositoryID string, from time.Time, to time.Time) error {
	s.organizationID = organizationID
	s.repositoryID = repositoryID
	s.from = from
	s.to = to
	s.calls++
	return nil
}

func TestConsumerRefreshesRepositoryFromEvent(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	event := syncjob.SyncCompletedEvent{
		OrganizationID: "bd546e60-e65d-b1fd-3713-6f56aa60f149",
		RepositoryID:   "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
		SyncJobID:      "job-1",
		OccurredAt:     to,
		EventType:      "repository.sync.completed",
		From:           from,
		To:             to,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	generator := &stubGenerator{}
	consumer := &Consumer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		generator: generator,
	}

	consumer.handleMessage(&nats.Msg{Data: payload})

	if generator.calls != 1 {
		t.Fatalf("expected one refresh call, got %d", generator.calls)
	}
	if generator.organizationID != event.OrganizationID {
		t.Fatalf("expected organization id %q, got %q", event.OrganizationID, generator.organizationID)
	}
	if generator.repositoryID != event.RepositoryID {
		t.Fatalf("expected repository id %q, got %q", event.RepositoryID, generator.repositoryID)
	}
	if !generator.from.Equal(from) {
		t.Fatalf("expected from %s, got %s", from, generator.from)
	}
	if !generator.to.Equal(to) {
		t.Fatalf("expected to %s, got %s", to, generator.to)
	}
}
