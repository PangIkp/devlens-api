package app

import (
	"context"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/syncjob"
)

type stubInsightRefresher struct {
	organizationID string
	repositoryID   string
	from           time.Time
	to             time.Time
	calls          int
}

func (s *stubInsightRefresher) RefreshRepository(_ context.Context, organizationID string, repositoryID string, from time.Time, to time.Time) error {
	s.organizationID = organizationID
	s.repositoryID = repositoryID
	s.from = from
	s.to = to
	s.calls++
	return nil
}

func TestInlineInsightPublisherRefreshesRepositoryWithFallbackDates(t *testing.T) {
	t.Parallel()

	refresher := &stubInsightRefresher{}
	publisher := inlineInsightPublisher{service: refresher}
	event := syncjob.SyncCompletedEvent{
		OrganizationID: "org-1",
		RepositoryID:   "repo-1",
		OccurredAt:     time.Date(2026, 8, 28, 16, 30, 0, 0, time.UTC),
	}

	if err := publisher.PublishRepositorySyncCompleted(context.Background(), event); err != nil {
		t.Fatalf("publish inline insight event: %v", err)
	}

	if refresher.calls != 1 {
		t.Fatalf("expected refresher to be called once, got %d", refresher.calls)
	}
	if refresher.organizationID != event.OrganizationID {
		t.Fatalf("expected organization %q, got %q", event.OrganizationID, refresher.organizationID)
	}
	if refresher.repositoryID != event.RepositoryID {
		t.Fatalf("expected repository %q, got %q", event.RepositoryID, refresher.repositoryID)
	}
	if refresher.from.Format("2006-01-02") != "1970-01-01" {
		t.Fatalf("expected fallback from date 1970-01-01, got %s", refresher.from.Format("2006-01-02"))
	}
	if !refresher.to.Equal(event.OccurredAt) {
		t.Fatalf("expected to=%s, got %s", event.OccurredAt, refresher.to)
	}
}
