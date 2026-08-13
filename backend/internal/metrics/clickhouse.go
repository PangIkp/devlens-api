package metrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5/pgtype"
)

type pullRequestRecord struct {
	ID           string  `json:"id"`
	RepositoryID string  `json:"repository_id"`
	GithubPRID   int64   `json:"github_pr_id"`
	Number       int32   `json:"number"`
	Title        string  `json:"title"`
	Author       string  `json:"author"`
	State        string  `json:"state"`
	CreatedAt    string  `json:"created_at"`
	MergedAt     *string `json:"merged_at"`
	ClosedAt     *string `json:"closed_at"`
	Additions    int32   `json:"additions"`
	Deletions    int32   `json:"deletions"`
	FilesChanged int32   `json:"files_changed"`
	IsDraft      bool    `json:"is_draft"`
	SyncedAt     string  `json:"synced_at"`
}

type pullRequestReviewRecord struct {
	ID                string  `json:"id"`
	PullRequestID     string  `json:"pull_request_id"`
	GithubReviewID    int64   `json:"github_review_id"`
	Reviewer          string  `json:"reviewer"`
	ReviewRequestedAt *string `json:"review_requested_at"`
	FirstReviewAt     *string `json:"first_review_at"`
	ReviewSubmittedAt *string `json:"review_submitted_at"`
	State             string  `json:"state"`
	SyncedAt          string  `json:"synced_at"`
}

type deploymentRecord struct {
	ID           string `json:"id"`
	RepositoryID string `json:"repository_id"`
	Environment  string `json:"environment"`
	Status       string `json:"status"`
	DeployedAt   string `json:"deployed_at"`
	SyncedAt     string `json:"synced_at"`
}

type commitEventRecord struct {
	ID              string  `json:"id"`
	RepositoryID    string  `json:"repository_id"`
	GitHubCommitSHA string  `json:"github_commit_sha"`
	Author          string  `json:"author"`
	AuthorEmail     *string `json:"author_email"`
	Message         string  `json:"message"`
	AuthoredAt      string  `json:"authored_at"`
	SyncedAt        string  `json:"synced_at"`
}

type workflowEventRecord struct {
	ID                string  `json:"id"`
	RepositoryID      string  `json:"repository_id"`
	GithubWorkflowRun int64   `json:"github_workflow_run_id"`
	WorkflowName      string  `json:"workflow_name"`
	Status            string  `json:"status"`
	Conclusion        *string `json:"conclusion"`
	StartedAt         *string `json:"started_at"`
	CompletedAt       *string `json:"completed_at"`
	SyncedAt          string  `json:"synced_at"`
}

type fileChangeRecord struct {
	ID            string `json:"id"`
	PullRequestID string `json:"pull_request_id"`
	FilePath      string `json:"file_path"`
	Additions     int32  `json:"additions"`
	Deletions     int32  `json:"deletions"`
	CommitCount   int32  `json:"commit_count"`
	SyncedAt      string `json:"synced_at"`
}

type metricsDailyRecord struct {
	MetricVersion             int64   `json:"metric_version"`
	RepositoryID              string  `json:"repository_id"`
	MetricDate                string  `json:"metric_date"`
	PRCycleTimeMinutes        float64 `json:"pr_cycle_time_minutes"`
	ReviewWaitMinutes         float64 `json:"review_wait_minutes"`
	AverageReviewMinutes      float64 `json:"average_review_minutes"`
	AverageFilesChanged       float64 `json:"average_files_changed"`
	AverageAdditions          float64 `json:"average_additions"`
	AverageDeletions          float64 `json:"average_deletions"`
	DeploymentFrequency       float64 `json:"deployment_frequency"`
	ChangeFailureRate         float64 `json:"change_failure_rate"`
	ReviewCoverage            float64 `json:"review_coverage"`
	PRCount                   int64   `json:"pr_count"`
	MergedPRCount             int64   `json:"merged_pr_count"`
	ReviewedPRCount           int64   `json:"reviewed_pr_count"`
	ReviewWaitSampleCount     int64   `json:"review_wait_sample_count"`
	ReviewTimeSampleCount     int64   `json:"review_time_sample_count"`
	SuccessfulDeploymentCount int64   `json:"successful_deployment_count"`
	FailedDeploymentCount     int64   `json:"failed_deployment_count"`
	CalculatedAt              string  `json:"calculated_at"`
}

type hotspotRow struct {
	FilePath    string `json:"file_path"`
	Additions   int32  `json:"additions"`
	Deletions   int32  `json:"deletions"`
	CommitCount int32  `json:"commit_count"`
}

func (s *Service) syncAnalyticsRawData(ctx context.Context, repositoryID pgtype.UUID, bounds dateBounds, syncedAt time.Time) error {
	prs, err := s.pg.Queries().ListPullRequestsForAnalytics(ctx, sqlcgen.ListPullRequestsForAnalyticsParams{
		RepositoryID: repositoryID,
		MergedAt:     toTimestamp(bounds.From),
		CreatedAt:    toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return fmt.Errorf("list pull requests for analytics: %w", err)
	}

	prPayload := make([]pullRequestRecord, 0, len(prs))
	for _, item := range prs {
		prPayload = append(prPayload, pullRequestRecord{
			ID:           item.ID.String(),
			RepositoryID: item.RepositoryID.String(),
			GithubPRID:   item.GithubPrID,
			Number:       item.Number,
			Title:        item.Title,
			Author:       item.Author,
			State:        item.State,
			CreatedAt:    formatTimestamp(item.CreatedAt.Time),
			MergedAt:     nullableTimestamp(item.MergedAt),
			ClosedAt:     nullableTimestamp(item.ClosedAt),
			Additions:    item.Additions,
			Deletions:    item.Deletions,
			FilesChanged: item.FilesChanged,
			IsDraft:      item.IsDraft,
			SyncedAt:     formatTimestamp(syncedAt),
		})
	}
	if err := s.ch.InsertJSONEachRow(ctx, "INSERT INTO pull_requests", prPayload); err != nil {
		return fmt.Errorf("sync clickhouse pull_requests: %w", err)
	}

	reviews, err := s.pg.Queries().ListPullRequestReviewsForAnalytics(ctx, sqlcgen.ListPullRequestReviewsForAnalyticsParams{
		RepositoryID: repositoryID,
		MergedAt:     toTimestamp(bounds.From),
		CreatedAt:    toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return fmt.Errorf("list pull request reviews for analytics: %w", err)
	}

	reviewPayload := make([]pullRequestReviewRecord, 0, len(reviews))
	for _, item := range reviews {
		reviewPayload = append(reviewPayload, pullRequestReviewRecord{
			ID:                item.ID.String(),
			PullRequestID:     item.PullRequestID.String(),
			GithubReviewID:    item.GithubReviewID,
			Reviewer:          item.Reviewer,
			ReviewRequestedAt: nullableTimestamp(item.ReviewRequestedAt),
			FirstReviewAt:     nullableTimestamp(item.FirstReviewAt),
			ReviewSubmittedAt: nullableTimestamp(item.ReviewSubmittedAt),
			State:             item.State,
			SyncedAt:          formatTimestamp(syncedAt),
		})
	}
	if err := s.ch.InsertJSONEachRow(ctx, "INSERT INTO pull_request_reviews", reviewPayload); err != nil {
		return fmt.Errorf("sync clickhouse pull_request_reviews: %w", err)
	}

	deployments, err := s.pg.Queries().ListDeploymentsForAnalytics(ctx, sqlcgen.ListDeploymentsForAnalyticsParams{
		RepositoryID: repositoryID,
		DeployedAt:   toTimestamp(bounds.From),
		DeployedAt_2: toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return fmt.Errorf("list deployments for analytics: %w", err)
	}

	deploymentPayload := make([]deploymentRecord, 0, len(deployments))
	for _, item := range deployments {
		deploymentPayload = append(deploymentPayload, deploymentRecord{
			ID:           item.ID.String(),
			RepositoryID: item.RepositoryID.String(),
			Environment:  item.Environment,
			Status:       item.Status,
			DeployedAt:   formatTimestamp(item.DeployedAt.Time),
			SyncedAt:     formatTimestamp(syncedAt),
		})
	}
	if err := s.ch.InsertJSONEachRow(ctx, "INSERT INTO deployments", deploymentPayload); err != nil {
		return fmt.Errorf("sync clickhouse deployments: %w", err)
	}

	commitRows, err := s.listCommitEventsForAnalytics(ctx, repositoryID, bounds)
	if err != nil {
		return err
	}

	commitPayload := make([]commitEventRecord, 0, len(commitRows))
	for _, item := range commitRows {
		commitPayload = append(commitPayload, commitEventRecord{
			ID:              item.ID.String(),
			RepositoryID:    item.RepositoryID.String(),
			GitHubCommitSHA: item.GithubCommitSHA,
			Author:          item.Author,
			AuthorEmail:     nullableText(item.AuthorEmail),
			Message:         item.Message,
			AuthoredAt:      formatTimestamp(item.AuthoredAt.Time),
			SyncedAt:        formatTimestamp(syncedAt),
		})
	}
	if err := s.ch.InsertJSONEachRow(ctx, "INSERT INTO commit_events", commitPayload); err != nil {
		return fmt.Errorf("sync clickhouse commit_events: %w", err)
	}

	workflowRows, err := s.listWorkflowEventsForAnalytics(ctx, repositoryID, bounds)
	if err != nil {
		return err
	}

	workflowPayload := make([]workflowEventRecord, 0, len(workflowRows))
	for _, item := range workflowRows {
		workflowPayload = append(workflowPayload, workflowEventRecord{
			ID:                item.ID.String(),
			RepositoryID:      item.RepositoryID.String(),
			GithubWorkflowRun: item.GithubWorkflowRunID,
			WorkflowName:      item.WorkflowName,
			Status:            item.Status,
			Conclusion:        nullableText(item.Conclusion),
			StartedAt:         nullableTimestamp(item.StartedAt),
			CompletedAt:       nullableTimestamp(item.CompletedAt),
			SyncedAt:          formatTimestamp(syncedAt),
		})
	}
	if err := s.ch.InsertJSONEachRow(ctx, "INSERT INTO workflow_events", workflowPayload); err != nil {
		return fmt.Errorf("sync clickhouse workflow_events: %w", err)
	}

	fileChanges, err := s.pg.Queries().ListFileChangesForAnalytics(ctx, sqlcgen.ListFileChangesForAnalyticsParams{
		RepositoryID: repositoryID,
		MergedAt:     toTimestamp(bounds.From),
		CreatedAt:    toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return fmt.Errorf("list file_changes for analytics: %w", err)
	}

	filePayload := make([]fileChangeRecord, 0, len(fileChanges))
	for _, item := range fileChanges {
		filePayload = append(filePayload, fileChangeRecord{
			ID:            item.ID.String(),
			PullRequestID: item.PullRequestID.String(),
			FilePath:      item.FilePath,
			Additions:     item.Additions,
			Deletions:     item.Deletions,
			CommitCount:   item.CommitCount,
			SyncedAt:      formatTimestamp(syncedAt),
		})
	}
	if err := s.ch.InsertJSONEachRow(ctx, "INSERT INTO file_changes", filePayload); err != nil {
		return fmt.Errorf("sync clickhouse file_changes: %w", err)
	}

	return nil
}

type workflowEventAnalyticsRow struct {
	ID                  pgtype.UUID
	RepositoryID        pgtype.UUID
	GithubWorkflowRunID int64
	WorkflowName        string
	Status              string
	Conclusion          pgtype.Text
	StartedAt           pgtype.Timestamptz
	CompletedAt         pgtype.Timestamptz
}

func (s *Service) listWorkflowEventsForAnalytics(ctx context.Context, repositoryID pgtype.UUID, bounds dateBounds) ([]workflowEventAnalyticsRow, error) {
	rows, err := s.pg.Pool().Query(ctx, `
SELECT id,
       repository_id,
       github_workflow_run_id,
       workflow_name,
       status,
       conclusion,
       started_at,
       completed_at
FROM workflow_events
WHERE repository_id = $1
  AND coalesce(started_at, completed_at, updated_at, created_at) >= $2
  AND coalesce(started_at, completed_at, updated_at, created_at) < $3
ORDER BY coalesce(started_at, completed_at, updated_at, created_at) ASC`,
		repositoryID,
		toTimestamp(bounds.From),
		toTimestamp(bounds.ToExclusive),
	)
	if err != nil {
		return nil, fmt.Errorf("list workflow_events for analytics: %w", err)
	}
	defer rows.Close()

	items := make([]workflowEventAnalyticsRow, 0)
	for rows.Next() {
		var item workflowEventAnalyticsRow
		if err := rows.Scan(
			&item.ID,
			&item.RepositoryID,
			&item.GithubWorkflowRunID,
			&item.WorkflowName,
			&item.Status,
			&item.Conclusion,
			&item.StartedAt,
			&item.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan workflow_events analytics row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow_events analytics rows: %w", err)
	}
	return items, nil
}

type commitEventAnalyticsRow struct {
	ID              pgtype.UUID
	RepositoryID    pgtype.UUID
	GithubCommitSHA string
	Author          string
	AuthorEmail     pgtype.Text
	Message         string
	AuthoredAt      pgtype.Timestamptz
}

func (s *Service) listCommitEventsForAnalytics(ctx context.Context, repositoryID pgtype.UUID, bounds dateBounds) ([]commitEventAnalyticsRow, error) {
	rows, err := s.pg.Pool().Query(ctx, `
SELECT id,
       repository_id,
       github_commit_sha,
       author,
       author_email,
       message,
       authored_at
FROM commit_events
WHERE repository_id = $1
  AND authored_at >= $2
  AND authored_at < $3
ORDER BY authored_at ASC`,
		repositoryID,
		toTimestamp(bounds.From),
		toTimestamp(bounds.ToExclusive),
	)
	if err != nil {
		return nil, fmt.Errorf("list commit_events for analytics: %w", err)
	}
	defer rows.Close()

	items := make([]commitEventAnalyticsRow, 0)
	for rows.Next() {
		var item commitEventAnalyticsRow
		if err := rows.Scan(
			&item.ID,
			&item.RepositoryID,
			&item.GithubCommitSHA,
			&item.Author,
			&item.AuthorEmail,
			&item.Message,
			&item.AuthoredAt,
		); err != nil {
			return nil, fmt.Errorf("scan commit_events analytics row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commit_events analytics rows: %w", err)
	}
	return items, nil
}

func nullableText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func (s *Service) listMetricsDaily(ctx context.Context, repositoryID string, bounds dateBounds) ([]metricsDailyRecord, error) {
	query := fmt.Sprintf(`
SELECT
  metric_version,
  repository_id,
  toString(metric_date) AS metric_date,
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
  toString(calculated_at) AS calculated_at
FROM metrics_daily
FINAL
WHERE repository_id = '%s'
  AND metric_date >= toDate('%s')
  AND metric_date <= toDate('%s')
ORDER BY metric_date ASC
`, escapeString(repositoryID), formatDate(bounds.From), formatDate(bounds.ToInclusive))

	rows, err := clickhouse.QueryJSONEachRow[metricsDailyRecord](ctx, s.ch, query)
	if err != nil {
		return nil, fmt.Errorf("query metrics_daily: %w", err)
	}
	return rows, nil
}

func (s *Service) listDeployments(ctx context.Context, repositoryID string, bounds dateBounds, environment string) ([]deploymentRecord, error) {
	query := fmt.Sprintf(`
SELECT
  id,
  repository_id,
  environment,
  status,
  toString(deployed_at) AS deployed_at,
  toString(synced_at) AS synced_at
FROM deployments
FINAL
WHERE repository_id = '%s'
  AND environment = '%s'
  AND deployed_at >= toDateTime64('%s', 3, 'UTC')
  AND deployed_at < toDateTime64('%s', 3, 'UTC')
ORDER BY deployed_at ASC
`, escapeString(repositoryID), escapeString(environment), formatTimestamp(bounds.From), formatTimestamp(bounds.ToExclusive))

	rows, err := clickhouse.QueryJSONEachRow[deploymentRecord](ctx, s.ch, query)
	if err != nil {
		return nil, fmt.Errorf("query deployments: %w", err)
	}
	return rows, nil
}

func (s *Service) listHotspotRows(ctx context.Context, repositoryID string, bounds dateBounds) ([]hotspotRow, error) {
	query := fmt.Sprintf(`
SELECT
  fc.file_path,
  fc.additions,
  fc.deletions,
  fc.commit_count
FROM (
  SELECT
    pull_request_id,
    file_path,
    additions,
    deletions,
    commit_count
  FROM file_changes FINAL
) AS fc
INNER JOIN (
  SELECT
    id,
    repository_id,
    created_at
  FROM pull_requests FINAL
) AS pr ON pr.id = fc.pull_request_id
WHERE pr.repository_id = '%s'
  AND pr.created_at >= toDateTime64('%s', 3, 'UTC')
  AND pr.created_at < toDateTime64('%s', 3, 'UTC')
`, escapeString(repositoryID), formatTimestamp(bounds.From), formatTimestamp(bounds.ToExclusive))

	rows, err := clickhouse.QueryJSONEachRow[hotspotRow](ctx, s.ch, query)
	if err != nil {
		return nil, fmt.Errorf("query hotspots: %w", err)
	}
	return rows, nil
}

func aggregateSummary(repositoryID string, bounds dateBounds, dayType string, rows []metricsDailyRecord) DashboardSummary {
	summary := DashboardSummary{
		MetricVersion: CurrentMetricVersion,
		DayType:       dayType,
		RepositoryID:  repositoryID,
		From:          formatDate(bounds.From),
		To:            formatDate(bounds.ToInclusive),
	}

	var totalCycleWeighted float64
	var totalMergedCount int64
	var totalWaitWeighted float64
	var totalWaitSamples int64
	var totalSuccessful int64
	var totalFailed int64
	var totalReviewed int64
	var totalPRCount int64

	for _, row := range rows {
		totalCycleWeighted += row.PRCycleTimeMinutes * float64(row.MergedPRCount)
		totalMergedCount += row.MergedPRCount
		totalWaitWeighted += row.ReviewWaitMinutes * float64(row.ReviewWaitSampleCount)
		totalWaitSamples += row.ReviewWaitSampleCount
		totalSuccessful += row.SuccessfulDeploymentCount
		totalFailed += row.FailedDeploymentCount
		totalReviewed += row.ReviewedPRCount
		totalPRCount += row.PRCount
	}

	if totalMergedCount > 0 {
		summary.PRCycleTimeMinutes = totalCycleWeighted / float64(totalMergedCount)
	}
	if totalWaitSamples > 0 {
		summary.ReviewWaitMinutes = totalWaitWeighted / float64(totalWaitSamples)
	}
	if dayCount := bounds.dayCount(dayType); dayCount > 0 {
		summary.DeploymentFrequency = float64(totalSuccessful) / float64(dayCount)
	}
	if totalDeployments := totalSuccessful + totalFailed; totalDeployments > 0 {
		summary.ChangeFailureRate = float64(totalFailed) / float64(totalDeployments)
	}
	if totalPRCount > 0 {
		summary.ReviewCoverage = float64(totalReviewed) / float64(totalPRCount)
	}

	return summary
}

func aggregatePullRequestMetrics(bounds dateBounds, interval string, dayType string, rows []metricsDailyRecord) PullRequestMetrics {
	result := PullRequestMetrics{
		MetricVersion:  CurrentMetricVersion,
		DayType:        dayType,
		CycleTimeTrend: make([]MetricPoint, 0),
	}

	var totalCycleWeighted float64
	var totalMergedCount int64
	var totalFilesWeighted float64
	var totalAdditionsWeighted float64
	var totalDeletionsWeighted float64
	var totalPRCount int64

	for _, row := range rows {
		totalCycleWeighted += row.PRCycleTimeMinutes * float64(row.MergedPRCount)
		totalMergedCount += row.MergedPRCount
		totalFilesWeighted += row.AverageFilesChanged * float64(row.PRCount)
		totalAdditionsWeighted += row.AverageAdditions * float64(row.PRCount)
		totalDeletionsWeighted += row.AverageDeletions * float64(row.PRCount)
		totalPRCount += row.PRCount
	}

	if totalMergedCount > 0 {
		result.AverageCycleTimeMinutes = totalCycleWeighted / float64(totalMergedCount)
	}
	if totalPRCount > 0 {
		result.AverageFilesChanged = totalFilesWeighted / float64(totalPRCount)
		result.AverageAdditions = totalAdditionsWeighted / float64(totalPRCount)
		result.AverageDeletions = totalDeletionsWeighted / float64(totalPRCount)
	}

	trends := groupMetricsByInterval(bounds, interval, rows)
	for _, item := range trends {
		result.CycleTimeTrend = append(result.CycleTimeTrend, MetricPoint{
			Date:  item.Label,
			Value: weightedAverage(item.CycleWeighted, item.MergedCount),
		})
	}

	return result
}

func aggregateReviewMetrics(bounds dateBounds, interval string, dayType string, rows []metricsDailyRecord) ReviewMetrics {
	result := ReviewMetrics{
		MetricVersion: CurrentMetricVersion,
		DayType:       dayType,
		WaitTimeTrend: make([]MetricPoint, 0),
	}

	var totalWaitWeighted float64
	var totalWaitSamples int64
	var totalReviewWeighted float64
	var totalReviewSamples int64
	var totalReviewed int64
	var totalPRCount int64

	for _, row := range rows {
		totalWaitWeighted += row.ReviewWaitMinutes * float64(row.ReviewWaitSampleCount)
		totalWaitSamples += row.ReviewWaitSampleCount
		totalReviewWeighted += row.AverageReviewMinutes * float64(row.ReviewTimeSampleCount)
		totalReviewSamples += row.ReviewTimeSampleCount
		totalReviewed += row.ReviewedPRCount
		totalPRCount += row.PRCount
	}

	if totalWaitSamples > 0 {
		result.AverageWaitMinutes = totalWaitWeighted / float64(totalWaitSamples)
	}
	if totalReviewSamples > 0 {
		result.AverageReviewMinutes = totalReviewWeighted / float64(totalReviewSamples)
	}
	if totalPRCount > 0 {
		result.ReviewCoverage = float64(totalReviewed) / float64(totalPRCount)
	}

	trends := groupMetricsByInterval(bounds, interval, rows)
	for _, item := range trends {
		result.WaitTimeTrend = append(result.WaitTimeTrend, MetricPoint{
			Date:  item.Label,
			Value: weightedAverage(item.ReviewWaitWeighted, item.ReviewWaitCount),
		})
	}

	return result
}

func aggregateDeploymentMetrics(bounds dateBounds, interval string, dayType string, rows []metricsDailyRecord) DeploymentMetrics {
	result := DeploymentMetrics{
		MetricVersion:   CurrentMetricVersion,
		DayType:         dayType,
		DeploymentTrend: make([]MetricPoint, 0),
	}

	var totalSuccessful int64
	var totalFailed int64
	for _, row := range rows {
		totalSuccessful += row.SuccessfulDeploymentCount
		totalFailed += row.FailedDeploymentCount
	}

	result.DeploymentCount = int(totalSuccessful)
	if dayCount := bounds.dayCount(dayType); dayCount > 0 {
		result.DeploymentFrequency = float64(totalSuccessful) / float64(dayCount)
	}
	if totalDeployments := totalSuccessful + totalFailed; totalDeployments > 0 {
		result.ChangeFailureRate = float64(totalFailed) / float64(totalDeployments)
	}

	trends := groupMetricsByInterval(bounds, interval, rows)
	for _, item := range trends {
		result.DeploymentTrend = append(result.DeploymentTrend, MetricPoint{
			Date:  item.Label,
			Value: float64(item.SuccessfulDeployments),
		})
	}

	return result
}

func aggregateDeploymentMetricsFromRaw(bounds dateBounds, interval string, dayType string, rows []deploymentRecord) DeploymentMetrics {
	daily := make([]metricsDailyRecord, 0, bounds.DaysInclusive)
	byDay := make(map[string]*metricsDailyRecord, bounds.DaysInclusive)
	for day := bounds.From; !day.After(bounds.ToInclusive); day = day.AddDate(0, 0, 1) {
		key := formatDate(day)
		byDay[key] = &metricsDailyRecord{MetricDate: key}
	}

	for _, row := range rows {
		deployedAt, err := parseTimestamp(row.DeployedAt)
		if err != nil {
			continue
		}
		item := byDay[formatDate(deployedAt)]
		if strings.EqualFold(row.Status, "success") {
			item.SuccessfulDeploymentCount++
		}
		if strings.EqualFold(row.Status, "failed") {
			item.FailedDeploymentCount++
		}
	}

	for day := bounds.From; !day.After(bounds.ToInclusive); day = day.AddDate(0, 0, 1) {
		daily = append(daily, *byDay[formatDate(day)])
	}

	return aggregateDeploymentMetrics(bounds, interval, dayType, daily)
}

func aggregateHotspots(rows []hotspotRow, rules RuleConfig) []HotspotFile {
	byFile := make(map[string]*HotspotFile)
	for _, row := range rows {
		item, ok := byFile[row.FilePath]
		if !ok {
			item = &HotspotFile{FilePath: row.FilePath}
			byFile[row.FilePath] = item
		}
		item.Additions += int(row.Additions)
		item.Deletions += int(row.Deletions)
		item.CommitCount += int(row.CommitCount)
		item.HotspotScore = float64(item.CommitCount)*rules.HotspotCommitWeight +
			float64(item.Additions)*rules.HotspotAdditionsWeight +
			float64(item.Deletions)*rules.HotspotDeletionsWeight
	}

	files := make([]HotspotFile, 0, len(byFile))
	for _, item := range byFile {
		files = append(files, *item)
	}
	return files
}

type intervalAggregate struct {
	Label                 string
	CycleWeighted         float64
	MergedCount           int64
	ReviewWaitWeighted    float64
	ReviewWaitCount       int64
	SuccessfulDeployments int64
}

func groupMetricsByInterval(bounds dateBounds, interval string, rows []metricsDailyRecord) []intervalAggregate {
	index := make(map[string]*intervalAggregate)

	for day := bounds.From; !day.After(bounds.ToInclusive); day = day.AddDate(0, 0, 1) {
		label := intervalLabel(day, interval)
		if _, ok := index[label]; !ok {
			index[label] = &intervalAggregate{Label: label}
		}
	}

	for _, row := range rows {
		day, err := time.Parse("2006-01-02", row.MetricDate)
		if err != nil {
			continue
		}
		label := intervalLabel(day.UTC(), interval)
		item := index[label]
		item.CycleWeighted += row.PRCycleTimeMinutes * float64(row.MergedPRCount)
		item.MergedCount += row.MergedPRCount
		item.ReviewWaitWeighted += row.ReviewWaitMinutes * float64(row.ReviewWaitSampleCount)
		item.ReviewWaitCount += row.ReviewWaitSampleCount
		item.SuccessfulDeployments += row.SuccessfulDeploymentCount
	}

	items := make([]intervalAggregate, 0, len(index))
	for _, item := range index {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})
	return items
}

func intervalLabel(day time.Time, interval string) string {
	switch interval {
	case IntervalWeek:
		year, week := day.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week)
	case IntervalMonth:
		return day.Format("2006-01")
	default:
		return formatDate(day)
	}
}

func weightedAverage(total float64, count int64) float64 {
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func formatDate(value time.Time) string {
	return value.UTC().Format("2006-01-02")
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05.000")
}

func nullableTimestamp(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := formatTimestamp(value.Time)
	return &formatted
}

func parseTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02 15:04:05.000", value)
	if err == nil {
		return parsed.UTC(), nil
	}
	parsed, err = time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func escapeString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
