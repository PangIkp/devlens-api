package metrics

import (
	"testing"
	"time"
)

func TestAggregateSummaryWeightedAverages(t *testing.T) {
	t.Parallel()

	bounds, err := normalizeBounds(mustDate("2026-07-01"), mustDate("2026-07-02"))
	if err != nil {
		t.Fatalf("normalize bounds: %v", err)
	}

	rows := []metricsDailyRecord{
		{
			MetricDate:                "2026-07-01",
			PRCycleTimeMinutes:        60,
			MergedPRCount:             2,
			ReviewWaitMinutes:         30,
			ReviewWaitSampleCount:     2,
			SuccessfulDeploymentCount: 1,
			FailedDeploymentCount:     1,
			ReviewedPRCount:           1,
			PRCount:                   2,
		},
		{
			MetricDate:                "2026-07-02",
			PRCycleTimeMinutes:        120,
			MergedPRCount:             1,
			ReviewWaitMinutes:         90,
			ReviewWaitSampleCount:     1,
			SuccessfulDeploymentCount: 3,
			FailedDeploymentCount:     1,
			ReviewedPRCount:           3,
			PRCount:                   4,
		},
	}

	summary := aggregateSummary("repo-1", bounds, rows)
	if summary.PRCycleTimeMinutes != 80 {
		t.Fatalf("expected weighted pr cycle 80, got %v", summary.PRCycleTimeMinutes)
	}
	if summary.ReviewWaitMinutes != 50 {
		t.Fatalf("expected weighted review wait 50, got %v", summary.ReviewWaitMinutes)
	}
	if summary.DeploymentFrequency != 2 {
		t.Fatalf("expected deployment frequency 2, got %v", summary.DeploymentFrequency)
	}
	if summary.ChangeFailureRate != float64(2)/float64(6) {
		t.Fatalf("expected change failure rate 2/6, got %v", summary.ChangeFailureRate)
	}
	if summary.ReviewCoverage != float64(4)/float64(6) {
		t.Fatalf("unexpected review coverage %v", summary.ReviewCoverage)
	}
}

func TestAggregateHotspotsUsesCommitCountAndChurn(t *testing.T) {
	t.Parallel()

	files := aggregateHotspots([]hotspotRow{
		{FilePath: "internal/metrics/service.go", Additions: 10, Deletions: 5, CommitCount: 2},
		{FilePath: "internal/metrics/service.go", Additions: 3, Deletions: 2, CommitCount: 1},
	})

	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	if files[0].HotspotScore != 23 {
		t.Fatalf("expected hotspot score 23, got %v", files[0].HotspotScore)
	}
}

func mustDate(value string) (resultDate time.Time) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return parsed.UTC()
}
