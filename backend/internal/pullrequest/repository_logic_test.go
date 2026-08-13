package pullrequest

import (
	"testing"
	"time"
)

func TestBuildTimelineOrdersEvents(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	requestedAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	firstReviewAt := time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
	submittedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	mergedAt := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)

	events := buildTimeline("itsara", createdAt, &mergedAt, nil, []Review{
		{
			Reviewer:          "pangikp",
			State:             "approved",
			ReviewRequestedAt: &requestedAt,
			FirstReviewAt:     &firstReviewAt,
			ReviewSubmittedAt: &submittedAt,
		},
	})

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	expectedTypes := []string{"created", "review_requested", "review_started", "review_submitted", "merged"}
	for i, expected := range expectedTypes {
		if events[i].Type != expected {
			t.Fatalf("expected event %d type %q, got %q", i, expected, events[i].Type)
		}
	}
}

func TestBuildRiskIndicatorHighRisk(t *testing.T) {
	t.Parallel()

	risk := buildRiskIndicator(Response{
		Additions:    700,
		Deletions:    450,
		FilesChanged: 28,
		IsDraft:      false,
	}, nil, []FileChange{
		{CommitCount: 12},
	})

	if risk.Level != "high" {
		t.Fatalf("expected high risk, got %q", risk.Level)
	}

	expectedReasons := map[string]bool{
		"very_large_change":   true,
		"many_files_changed":  true,
		"no_reviews_recorded": true,
		"high_file_rework":    true,
	}
	for _, reason := range risk.Reasons {
		delete(expectedReasons, reason)
	}
	if len(expectedReasons) != 0 {
		t.Fatalf("missing expected reasons: %+v", expectedReasons)
	}
}

func TestBuildRiskIndicatorLowRisk(t *testing.T) {
	t.Parallel()

	submittedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	risk := buildRiskIndicator(Response{
		Additions:    40,
		Deletions:    10,
		FilesChanged: 2,
		IsDraft:      false,
	}, []Review{
		{Reviewer: "pangikp", State: "approved", ReviewSubmittedAt: &submittedAt},
	}, []FileChange{
		{CommitCount: 2},
	})

	if risk.Level != "low" {
		t.Fatalf("expected low risk, got %q", risk.Level)
	}
	if len(risk.Reasons) != 0 {
		t.Fatalf("expected no risk reasons, got %+v", risk.Reasons)
	}
}
