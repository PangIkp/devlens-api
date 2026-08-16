package sqlcgen

import (
	"strings"
	"testing"
)

func TestAggregateReviewMetricsByDayExcludesBotsAndInvalidTimestamps(t *testing.T) {
	t.Parallel()

	expectedSnippets := []string{
		"lower(btrim(rev.reviewer)) NOT LIKE '%[bot]'",
		"lower(btrim(rev.reviewer)) NOT LIKE 'dependabot%'",
		"lower(btrim(rev.reviewer)) NOT LIKE '%-bot'",
		"rev.first_review_at >= rev.review_requested_at",
		"rev.review_submitted_at >= rev.review_requested_at",
		"rev.review_requested_at >= pr.created_at",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(aggregateReviewMetricsByDay, snippet) {
			t.Fatalf("expected aggregateReviewMetricsByDay to contain %q", snippet)
		}
	}
}

func TestAggregatePRSizeByDayCountsOnlyNonBotReviews(t *testing.T) {
	t.Parallel()

	expectedSnippets := []string{
		"lower(btrim(reviewer)) NOT LIKE '%[bot]'",
		"lower(btrim(reviewer)) NOT LIKE 'dependabot%'",
		"lower(btrim(reviewer)) NOT LIKE '%-bot'",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(aggregatePRSizeByDay, snippet) {
			t.Fatalf("expected aggregatePRSizeByDay to contain %q", snippet)
		}
	}
}

func TestAggregatePRCycleByDayRejectsNegativeDurations(t *testing.T) {
	t.Parallel()

	if !strings.Contains(aggregatePRCycleByDay, "pr.merged_at >= pr.created_at") {
		t.Fatalf("expected aggregatePRCycleByDay to guard against negative cycle time")
	}
}

func TestListFileChangesForAnalyticsIncludesHistoricalOpenOrReopenedPullRequests(t *testing.T) {
	t.Parallel()

	expected := "(pr.merged_at IS NULL OR pr.merged_at >= $2 OR pr.created_at >= $2)"
	if !strings.Contains(listFileChangesForAnalytics, expected) {
		t.Fatalf("expected listFileChangesForAnalytics to contain %q", expected)
	}
}
