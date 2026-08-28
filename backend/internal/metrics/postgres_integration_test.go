package metrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUpsertMetricsDailyOverwritesExistingRowIntegration(t *testing.T) {
	t.Parallel()

	db := openIntegrationPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repositoryID := seedMetricsRepository(t, ctx, db)
	service := NewService(db, nil)

	firstCalculatedAt := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	secondCalculatedAt := firstCalculatedAt.Add(2 * time.Hour)

	err := service.upsertMetricsDaily(ctx, repositoryID, []metricsDailyRecord{{
		MetricVersion:             CurrentMetricVersion,
		MetricDate:                "2026-08-20",
		PRCycleTimeMinutes:        60,
		ReviewWaitMinutes:         30,
		AverageReviewMinutes:      45,
		AverageFilesChanged:       3,
		AverageAdditions:          10,
		AverageDeletions:          5,
		DeploymentFrequency:       1,
		ChangeFailureRate:         0.1,
		ReviewCoverage:            0.5,
		PRCount:                   2,
		MergedPRCount:             1,
		ReviewedPRCount:           1,
		ReviewWaitSampleCount:     1,
		ReviewTimeSampleCount:     1,
		SuccessfulDeploymentCount: 1,
		FailedDeploymentCount:     0,
		CalculatedAt:              firstCalculatedAt.Format(time.RFC3339),
	}})
	if err != nil {
		t.Fatalf("initial upsert metrics_daily: %v", err)
	}

	err = service.upsertMetricsDaily(ctx, repositoryID, []metricsDailyRecord{{
		MetricVersion:             CurrentMetricVersion,
		MetricDate:                "2026-08-20",
		PRCycleTimeMinutes:        120,
		ReviewWaitMinutes:         90,
		AverageReviewMinutes:      100,
		AverageFilesChanged:       7,
		AverageAdditions:          30,
		AverageDeletions:          12,
		DeploymentFrequency:       2,
		ChangeFailureRate:         0.25,
		ReviewCoverage:            1,
		PRCount:                   4,
		MergedPRCount:             2,
		ReviewedPRCount:           4,
		ReviewWaitSampleCount:     4,
		ReviewTimeSampleCount:     4,
		SuccessfulDeploymentCount: 2,
		FailedDeploymentCount:     1,
		CalculatedAt:              secondCalculatedAt.Format(time.RFC3339),
	}})
	if err != nil {
		t.Fatalf("overwrite upsert metrics_daily: %v", err)
	}

	var (
		rowCount         int
		prCount          int64
		cycleTime        float64
		changeFailRate   float64
		calculatedAtRead time.Time
	)
	if err := db.Pool().QueryRow(ctx, `
		SELECT COUNT(*), MAX(pr_count), MAX(pr_cycle_time_minutes), MAX(change_failure_rate), MAX(calculated_at)
		FROM metrics_daily
		WHERE repository_id = $1 AND metric_version = $2 AND metric_date = $3`,
		parseMetricsUUID(repositoryID),
		CurrentMetricVersion,
		time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
	).Scan(&rowCount, &prCount, &cycleTime, &changeFailRate, &calculatedAtRead); err != nil {
		t.Fatalf("query metrics_daily: %v", err)
	}

	if rowCount != 1 {
		t.Fatalf("expected 1 metrics_daily row, got %d", rowCount)
	}
	if prCount != 4 {
		t.Fatalf("expected overwritten pr_count 4, got %d", prCount)
	}
	if cycleTime != 120 {
		t.Fatalf("expected overwritten pr_cycle_time_minutes 120, got %v", cycleTime)
	}
	if changeFailRate != 0.25 {
		t.Fatalf("expected overwritten change_failure_rate 0.25, got %v", changeFailRate)
	}
	if !calculatedAtRead.Equal(secondCalculatedAt) {
		t.Fatalf("expected calculated_at %s, got %s", secondCalculatedAt, calculatedAtRead)
	}
}

func TestServiceReadsMetricsFromPostgresOnlyIntegration(t *testing.T) {
	t.Parallel()

	db := openIntegrationPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repositoryID := seedMetricsRepository(t, ctx, db)
	repositoryUUID := parseMetricsUUID(repositoryID)
	service := NewService(db, nil)

	calculatedAt := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	err := service.upsertMetricsDaily(ctx, repositoryID, []metricsDailyRecord{
		{
			MetricVersion:             CurrentMetricVersion,
			MetricDate:                "2026-08-20",
			PRCycleTimeMinutes:        60,
			ReviewWaitMinutes:         30,
			AverageReviewMinutes:      45,
			AverageFilesChanged:       3,
			AverageAdditions:          10,
			AverageDeletions:          5,
			DeploymentFrequency:       1,
			ChangeFailureRate:         0,
			ReviewCoverage:            0.5,
			PRCount:                   2,
			MergedPRCount:             2,
			ReviewedPRCount:           1,
			ReviewWaitSampleCount:     2,
			ReviewTimeSampleCount:     1,
			SuccessfulDeploymentCount: 1,
			FailedDeploymentCount:     0,
			CalculatedAt:              calculatedAt.Format(time.RFC3339),
		},
		{
			MetricVersion:             CurrentMetricVersion,
			MetricDate:                "2026-08-21",
			PRCycleTimeMinutes:        120,
			ReviewWaitMinutes:         90,
			AverageReviewMinutes:      135,
			AverageFilesChanged:       6,
			AverageAdditions:          30,
			AverageDeletions:          15,
			DeploymentFrequency:       0.5,
			ChangeFailureRate:         0.5,
			ReviewCoverage:            1,
			PRCount:                   1,
			MergedPRCount:             1,
			ReviewedPRCount:           1,
			ReviewWaitSampleCount:     1,
			ReviewTimeSampleCount:     1,
			SuccessfulDeploymentCount: 1,
			FailedDeploymentCount:     1,
			CalculatedAt:              calculatedAt.Format(time.RFC3339),
		},
	})
	if err != nil {
		t.Fatalf("seed metrics_daily rows: %v", err)
	}

	pullRequestID := newMetricsUUID()
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO pull_requests (
			id, repository_id, github_pr_id, number, title, author, state, created_at, merged_at, additions, deletions, files_changed, is_draft
		) VALUES (
			$1, $2, $3, 1, 'Metrics PR', 'itsara', 'closed', $4, $5, 40, 10, 2, FALSE
		)`,
		pullRequestID,
		repositoryUUID,
		time.Now().UTC().UnixNano(),
		time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 20, 15, 0, 0, 0, time.UTC),
	); err != nil {
		t.Fatalf("insert pull request: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO file_changes (id, pull_request_id, file_path, additions, deletions, commit_count)
		VALUES ($1, $2, 'backend/internal/metrics/service.go', 40, 10, 2)`,
		newMetricsUUID(),
		pullRequestID,
	); err != nil {
		t.Fatalf("insert file change: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO deployments (id, repository_id, github_deployment_id, environment, status, deployed_at)
		VALUES
			($1, $2, $3, 'production', 'success', $4),
			($5, $2, $6, 'staging', 'failed', $7)`,
		newMetricsUUID(),
		repositoryUUID,
		time.Now().UTC().UnixNano(),
		time.Date(2026, 8, 20, 18, 0, 0, 0, time.UTC),
		newMetricsUUID(),
		time.Now().UTC().UnixNano()+1,
		time.Date(2026, 8, 21, 18, 0, 0, 0, time.UTC),
	); err != nil {
		t.Fatalf("insert deployments: %v", err)
	}

	params := QueryParams{
		From:     time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		To:       time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		Interval: IntervalDay,
		DayType:  DayTypeCalendar,
	}

	summary, err := service.GetDashboardSummary(ctx, repositoryID, params)
	if err != nil {
		t.Fatalf("get dashboard summary: %v", err)
	}
	if summary.PRCycleTimeMinutes != 80 {
		t.Fatalf("expected summary pr cycle 80, got %v", summary.PRCycleTimeMinutes)
	}
	if summary.ReviewWaitMinutes != 50 {
		t.Fatalf("expected summary review wait 50, got %v", summary.ReviewWaitMinutes)
	}
	if summary.DeploymentFrequency != 1 {
		t.Fatalf("expected summary deployment frequency 1, got %v", summary.DeploymentFrequency)
	}

	pullRequests, err := service.GetPullRequestMetrics(ctx, repositoryID, params)
	if err != nil {
		t.Fatalf("get pull request metrics: %v", err)
	}
	if pullRequests.AverageCycleTimeMinutes != 80 {
		t.Fatalf("expected average cycle time 80, got %v", pullRequests.AverageCycleTimeMinutes)
	}
	if pullRequests.AverageFilesChanged != 4 {
		t.Fatalf("expected average files changed 4, got %v", pullRequests.AverageFilesChanged)
	}

	reviews, err := service.GetReviewMetrics(ctx, repositoryID, params)
	if err != nil {
		t.Fatalf("get review metrics: %v", err)
	}
	if reviews.AverageWaitMinutes != 50 {
		t.Fatalf("expected average wait 50, got %v", reviews.AverageWaitMinutes)
	}
	if reviews.AverageReviewMinutes != 90 {
		t.Fatalf("expected average review 90, got %v", reviews.AverageReviewMinutes)
	}

	deployments, err := service.GetDeploymentMetrics(ctx, repositoryID, DeploymentQueryParams{QueryParams: params})
	if err != nil {
		t.Fatalf("get deployment metrics: %v", err)
	}
	if deployments.DeploymentCount != 2 {
		t.Fatalf("expected deployment count 2, got %d", deployments.DeploymentCount)
	}
	if deployments.ChangeFailureRate != 1.0/3.0 {
		t.Fatalf("expected aggregate change failure rate 1/3, got %v", deployments.ChangeFailureRate)
	}

	production := "production"
	productionDeployments, err := service.GetDeploymentMetrics(ctx, repositoryID, DeploymentQueryParams{
		QueryParams: params,
		Environment: &production,
	})
	if err != nil {
		t.Fatalf("get production deployment metrics: %v", err)
	}
	if productionDeployments.DeploymentCount != 1 {
		t.Fatalf("expected 1 production deployment, got %d", productionDeployments.DeploymentCount)
	}
	if productionDeployments.ChangeFailureRate != 0 {
		t.Fatalf("expected production change failure rate 0, got %v", productionDeployments.ChangeFailureRate)
	}

	hotspots, err := service.GetHotspots(ctx, repositoryID, HotspotQueryParams{
		From:      params.From,
		To:        params.To,
		Page:      1,
		PageSize:  10,
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("get hotspots: %v", err)
	}
	if hotspots.TotalItems != 1 {
		t.Fatalf("expected 1 hotspot item, got %d", hotspots.TotalItems)
	}
	if hotspots.Items[0].HotspotScore != 52 {
		t.Fatalf("expected hotspot score 52, got %v", hotspots.Items[0].HotspotScore)
	}

	repositoryMetrics, err := service.GetRepositoryMetrics(ctx, repositoryID, DeploymentQueryParams{QueryParams: params})
	if err != nil {
		t.Fatalf("get repository metrics: %v", err)
	}
	if repositoryMetrics.Summary.PRCycleTimeMinutes != 80 {
		t.Fatalf("expected repository summary pr cycle 80, got %v", repositoryMetrics.Summary.PRCycleTimeMinutes)
	}
	if len(repositoryMetrics.Hotspots) != 1 {
		t.Fatalf("expected repository metrics hotspots, got %d", len(repositoryMetrics.Hotspots))
	}
}

func openIntegrationPostgres(t *testing.T) *postgres.DB {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Postgres.ConnectTimeout)
	defer cancel()

	db, err := postgres.Open(ctx, cfg.Postgres, nil)
	if err != nil {
		t.Skipf("skip integration test: postgres unavailable: %v", err)
	}
	t.Cleanup(db.Close)

	return db
}

func seedMetricsRepository(t *testing.T, ctx context.Context, db *postgres.DB) string {
	t.Helper()

	orgID := newMetricsUUID()
	repositoryID := newMetricsUUID()
	suffix := time.Now().UTC().UnixNano()

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO organizations (id, github_id, name, slug, created_at)
		VALUES ($1, $2, $3, $4, NOW())`,
		orgID,
		suffix,
		"Metrics Integration Org",
		fmt.Sprintf("metrics-int-%d", suffix),
	); err != nil {
		t.Fatalf("insert organization: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active)
		VALUES ($1, $2, $3, $4, $5, 'main', TRUE)`,
		repositoryID,
		orgID,
		suffix,
		"metrics-repo",
		fmt.Sprintf("devlens/metrics-repo-%d", suffix),
	); err != nil {
		t.Fatalf("insert repository: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	return repositoryID.String()
}

func newMetricsUUID() pgtype.UUID {
	return parseMetricsUUID(fmt.Sprintf("00000000-0000-0000-0000-%012x", time.Now().UTC().UnixNano()&0xffffffffffff))
}

func parseMetricsUUID(value string) pgtype.UUID {
	var parsed pgtype.UUID
	if err := parsed.Scan(value); err != nil {
		panic(err)
	}
	return parsed
}
