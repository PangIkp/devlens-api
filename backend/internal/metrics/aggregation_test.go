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

	summary := aggregateSummary("repo-1", bounds, DayTypeCalendar, rows)
	if summary.MetricVersion != CurrentMetricVersion {
		t.Fatalf("expected metric version %d, got %d", CurrentMetricVersion, summary.MetricVersion)
	}
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

func TestAggregateDeploymentMetricsUsesBusinessDayDenominator(t *testing.T) {
	t.Parallel()

	bounds, err := normalizeBounds(mustDate("2026-07-03"), mustDate("2026-07-06"))
	if err != nil {
		t.Fatalf("normalize bounds: %v", err)
	}

	rows := []metricsDailyRecord{
		{MetricDate: "2026-07-03", SuccessfulDeploymentCount: 2},
		{MetricDate: "2026-07-04", SuccessfulDeploymentCount: 1},
		{MetricDate: "2026-07-05", SuccessfulDeploymentCount: 1},
		{MetricDate: "2026-07-06", SuccessfulDeploymentCount: 2},
	}

	calendar := aggregateDeploymentMetrics(bounds, IntervalDay, DayTypeCalendar, rows)
	if calendar.DayType != DayTypeCalendar {
		t.Fatalf("expected calendar dayType, got %q", calendar.DayType)
	}
	if calendar.DeploymentFrequency != 1.5 {
		t.Fatalf("expected calendar deployment frequency 1.5, got %v", calendar.DeploymentFrequency)
	}

	business := aggregateDeploymentMetrics(bounds, IntervalDay, DayTypeBusiness, rows)
	if business.DayType != DayTypeBusiness {
		t.Fatalf("expected business dayType, got %q", business.DayType)
	}
	if business.DeploymentFrequency != 3 {
		t.Fatalf("expected business deployment frequency 3, got %v", business.DeploymentFrequency)
	}
}

func TestAggregateHotspotsUsesCommitCountAndChurn(t *testing.T) {
	t.Parallel()

	files := aggregateHotspots([]hotspotRow{
		{FilePath: "internal/metrics/service.go", Additions: 10, Deletions: 5, CommitCount: 2},
		{FilePath: "internal/metrics/service.go", Additions: 3, Deletions: 2, CommitCount: 1},
	}, DefaultRuleConfig())

	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	if files[0].HotspotScore != 23 {
		t.Fatalf("expected hotspot score 23, got %v", files[0].HotspotScore)
	}
}

func TestAggregateHotspotsAppliesConfiguredWeights(t *testing.T) {
	t.Parallel()

	files := aggregateHotspots([]hotspotRow{
		{FilePath: "internal/metrics/service.go", Additions: 10, Deletions: 5, CommitCount: 2},
	}, RuleConfig{
		DefaultDayType:         DayTypeCalendar,
		HotspotCommitWeight:    3,
		HotspotAdditionsWeight: 0.5,
		HotspotDeletionsWeight: 2,
	})

	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	expected := float64(2)*3 + float64(10)*0.5 + float64(5)*2
	if files[0].HotspotScore != expected {
		t.Fatalf("expected hotspot score %v, got %v", expected, files[0].HotspotScore)
	}
}

func mustDate(value string) (resultDate time.Time) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return parsed.UTC()
}
