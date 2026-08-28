package metrics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) upsertMetricsDaily(ctx context.Context, repositoryID string, rows []metricsDailyRecord) error {
	if s.pg == nil {
		return fmt.Errorf("metrics postgres dependency is not configured")
	}
	if len(rows) == 0 {
		return nil
	}

	repositoryUUID := parseUUID(repositoryID)
	if !repositoryUUID.Valid {
		return &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "repositoryId", Message: "must be a valid UUID"}},
		}
	}

	batch := s.pg.Pool().SendBatch(ctx, buildMetricsDailyBatch(repositoryUUID, rows))

	for range rows {
		if _, err := batch.Exec(); err != nil {
			_ = batch.Close()
			return fmt.Errorf("upsert metrics_daily rows: %w", err)
		}
	}
	if err := batch.Close(); err != nil {
		return fmt.Errorf("close metrics_daily batch: %w", err)
	}
	return nil
}

func buildMetricsDailyBatch(repositoryID pgtype.UUID, rows []metricsDailyRecord) *pgx.Batch {
	batch := &pgx.Batch{}
	for _, row := range rows {
		metricDate, _ := time.Parse("2006-01-02", row.MetricDate)
		calculatedAt, _ := time.Parse(time.RFC3339Nano, normalizeMetricTimestamp(row.CalculatedAt))
		batch.Queue(`
INSERT INTO metrics_daily (
	repository_id,
	metric_version,
	metric_date,
	pr_cycle_time_minutes,
	review_wait_minutes,
	average_review_minutes,
	average_files_changed,
	average_additions,
	average_deletions,
	deployment_frequency,
	change_failure_rate,
	review_coverage,
	pr_count,
	merged_pr_count,
	reviewed_pr_count,
	review_wait_sample_count,
	review_time_sample_count,
	successful_deployment_count,
	failed_deployment_count,
	calculated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
)
ON CONFLICT (repository_id, metric_version, metric_date) DO UPDATE SET
	pr_cycle_time_minutes = EXCLUDED.pr_cycle_time_minutes,
	review_wait_minutes = EXCLUDED.review_wait_minutes,
	average_review_minutes = EXCLUDED.average_review_minutes,
	average_files_changed = EXCLUDED.average_files_changed,
	average_additions = EXCLUDED.average_additions,
	average_deletions = EXCLUDED.average_deletions,
	deployment_frequency = EXCLUDED.deployment_frequency,
	change_failure_rate = EXCLUDED.change_failure_rate,
	review_coverage = EXCLUDED.review_coverage,
	pr_count = EXCLUDED.pr_count,
	merged_pr_count = EXCLUDED.merged_pr_count,
	reviewed_pr_count = EXCLUDED.reviewed_pr_count,
	review_wait_sample_count = EXCLUDED.review_wait_sample_count,
	review_time_sample_count = EXCLUDED.review_time_sample_count,
	successful_deployment_count = EXCLUDED.successful_deployment_count,
	failed_deployment_count = EXCLUDED.failed_deployment_count,
	calculated_at = EXCLUDED.calculated_at`,
			repositoryID,
			int32(row.MetricVersion),
			metricDate.UTC(),
			row.PRCycleTimeMinutes,
			row.ReviewWaitMinutes,
			row.AverageReviewMinutes,
			row.AverageFilesChanged,
			row.AverageAdditions,
			row.AverageDeletions,
			row.DeploymentFrequency,
			row.ChangeFailureRate,
			row.ReviewCoverage,
			row.PRCount,
			row.MergedPRCount,
			row.ReviewedPRCount,
			row.ReviewWaitSampleCount,
			row.ReviewTimeSampleCount,
			row.SuccessfulDeploymentCount,
			row.FailedDeploymentCount,
			calculatedAt.UTC(),
		)
	}
	return batch
}

func normalizeMetricTimestamp(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	if strings.HasSuffix(trimmed, "Z") {
		return trimmed
	}
	return trimmed + "Z"
}
