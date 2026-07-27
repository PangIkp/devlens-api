package metrics

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrRepositoryNotFound = errors.New("repository not found")

type Service struct {
	pg *postgres.DB
	ch *clickhouse.DB
}

func NewService(pg *postgres.DB, ch *clickhouse.DB) *Service {
	return &Service{pg: pg, ch: ch}
}

func (s *Service) CalculateRepositoryMetrics(ctx context.Context, repositoryID string, req CalculationRequest) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	bounds, err := normalizeBounds(req.From, req.To)
	if err != nil {
		return err
	}

	repoUUID, err := s.ensureRepositoryExists(ctx, repositoryID)
	if err != nil {
		return err
	}

	rows, err := s.buildDailyMetrics(ctx, repoUUID, bounds)
	if err != nil {
		return err
	}

	calculatedAt := time.Now().UTC()
	if err := s.syncAnalyticsRawData(ctx, repoUUID, bounds, calculatedAt); err != nil {
		return err
	}

	payload := make([]metricsDailyRecord, 0, len(rows))
	for _, row := range rows {
		payload = append(payload, row.toClickHouseRecord(repositoryID, calculatedAt))
	}

	if err := s.ch.InsertJSONEachRow(ctx, "INSERT INTO metrics_daily", payload); err != nil {
		return fmt.Errorf("insert metrics_daily rows: %w", err)
	}

	return nil
}

func (s *Service) GetDashboardSummary(ctx context.Context, repositoryID string, params QueryParams) (DashboardSummary, error) {
	if err := s.ensureReady(); err != nil {
		return DashboardSummary{}, err
	}

	bounds, err := normalizeBounds(params.From, params.To)
	if err != nil {
		return DashboardSummary{}, err
	}

	if _, err := s.ensureRepositoryExists(ctx, repositoryID); err != nil {
		return DashboardSummary{}, err
	}

	rows, err := s.listMetricsDaily(ctx, repositoryID, bounds)
	if err != nil {
		return DashboardSummary{}, err
	}

	summary := aggregateSummary(repositoryID, bounds, rows)
	return summary, nil
}

func (s *Service) GetPullRequestMetrics(ctx context.Context, repositoryID string, params QueryParams) (PullRequestMetrics, error) {
	if err := s.ensureReady(); err != nil {
		return PullRequestMetrics{}, err
	}

	bounds, err := normalizeBounds(params.From, params.To)
	if err != nil {
		return PullRequestMetrics{}, err
	}

	interval, err := normalizeInterval(params.Interval)
	if err != nil {
		return PullRequestMetrics{}, err
	}

	if _, err := s.ensureRepositoryExists(ctx, repositoryID); err != nil {
		return PullRequestMetrics{}, err
	}

	rows, err := s.listMetricsDaily(ctx, repositoryID, bounds)
	if err != nil {
		return PullRequestMetrics{}, err
	}

	return aggregatePullRequestMetrics(bounds, interval, rows), nil
}

func (s *Service) GetReviewMetrics(ctx context.Context, repositoryID string, params QueryParams) (ReviewMetrics, error) {
	if err := s.ensureReady(); err != nil {
		return ReviewMetrics{}, err
	}

	bounds, err := normalizeBounds(params.From, params.To)
	if err != nil {
		return ReviewMetrics{}, err
	}

	interval, err := normalizeInterval(params.Interval)
	if err != nil {
		return ReviewMetrics{}, err
	}

	if _, err := s.ensureRepositoryExists(ctx, repositoryID); err != nil {
		return ReviewMetrics{}, err
	}

	rows, err := s.listMetricsDaily(ctx, repositoryID, bounds)
	if err != nil {
		return ReviewMetrics{}, err
	}

	return aggregateReviewMetrics(bounds, interval, rows), nil
}

func (s *Service) GetDeploymentMetrics(ctx context.Context, repositoryID string, params DeploymentQueryParams) (DeploymentMetrics, error) {
	if err := s.ensureReady(); err != nil {
		return DeploymentMetrics{}, err
	}

	bounds, err := normalizeBounds(params.From, params.To)
	if err != nil {
		return DeploymentMetrics{}, err
	}

	interval, err := normalizeInterval(params.Interval)
	if err != nil {
		return DeploymentMetrics{}, err
	}

	if _, err := s.ensureRepositoryExists(ctx, repositoryID); err != nil {
		return DeploymentMetrics{}, err
	}

	if params.Environment != nil && strings.TrimSpace(*params.Environment) != "" {
		deployments, err := s.listDeployments(ctx, repositoryID, bounds, strings.TrimSpace(*params.Environment))
		if err != nil {
			return DeploymentMetrics{}, err
		}
		return aggregateDeploymentMetricsFromRaw(bounds, interval, deployments), nil
	}

	rows, err := s.listMetricsDaily(ctx, repositoryID, bounds)
	if err != nil {
		return DeploymentMetrics{}, err
	}

	return aggregateDeploymentMetrics(bounds, interval, rows), nil
}

func (s *Service) GetHotspots(ctx context.Context, repositoryID string, params HotspotQueryParams) (HotspotResult, error) {
	if err := s.ensureReady(); err != nil {
		return HotspotResult{}, err
	}

	bounds, err := normalizeBounds(params.From, params.To)
	if err != nil {
		return HotspotResult{}, err
	}

	if _, err := s.ensureRepositoryExists(ctx, repositoryID); err != nil {
		return HotspotResult{}, err
	}

	order := strings.ToLower(strings.TrimSpace(params.SortOrder))
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		return HotspotResult{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "sortOrder", Message: "must be one of asc, desc"}},
		}
	}
	if params.Page < 1 || params.PageSize < 1 {
		return HotspotResult{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "page", Message: "must be greater than or equal to 1"}},
		}
	}

	rows, err := s.listHotspotRows(ctx, repositoryID, bounds)
	if err != nil {
		return HotspotResult{}, err
	}

	files := aggregateHotspots(rows)
	sort.Slice(files, func(i, j int) bool {
		if files[i].HotspotScore == files[j].HotspotScore {
			if order == "asc" {
				return files[i].FilePath < files[j].FilePath
			}
			return files[i].FilePath < files[j].FilePath
		}
		if order == "asc" {
			return files[i].HotspotScore < files[j].HotspotScore
		}
		return files[i].HotspotScore > files[j].HotspotScore
	})

	totalItems := len(files)
	start := (params.Page - 1) * params.PageSize
	if start >= totalItems {
		return HotspotResult{Items: []HotspotFile{}, TotalItems: totalItems}, nil
	}

	end := start + params.PageSize
	if end > totalItems {
		end = totalItems
	}

	return HotspotResult{Items: files[start:end], TotalItems: totalItems}, nil
}

func (s *Service) ensureReady() error {
	if s.pg == nil {
		return fmt.Errorf("metrics postgres dependency is not configured")
	}
	if s.ch == nil {
		return fmt.Errorf("metrics clickhouse dependency is not configured")
	}
	return nil
}

func (s *Service) ensureRepositoryExists(ctx context.Context, repositoryID string) (pgtype.UUID, error) {
	id := parseUUID(repositoryID)
	if !id.Valid {
		return pgtype.UUID{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "repositoryId", Message: "must be a valid UUID"}},
		}
	}

	_, err := s.pg.Queries().GetRepositoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, ErrRepositoryNotFound
		}
		return pgtype.UUID{}, fmt.Errorf("get repository: %w", err)
	}

	return id, nil
}

func normalizeBounds(from time.Time, to time.Time) (dateBounds, error) {
	if from.IsZero() || to.IsZero() {
		return dateBounds{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{
				{Field: "from", Message: "is required"},
				{Field: "to", Message: "is required"},
			},
		}
	}

	start := time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.UTC().Year(), to.UTC().Month(), to.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if end.Before(start) {
		return dateBounds{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "to", Message: "must be greater than or equal to from"}},
		}
	}

	return dateBounds{
		From:          start,
		ToInclusive:   end,
		ToExclusive:   end.AddDate(0, 0, 1),
		DaysInclusive: int(end.Sub(start).Hours()/24) + 1,
	}, nil
}

func normalizeInterval(value string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return IntervalDay, nil
	}

	switch trimmed {
	case IntervalDay, IntervalWeek, IntervalMonth:
		return trimmed, nil
	default:
		return "", &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "interval", Message: "must be one of day, week, month"}},
		}
	}
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func toTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

type dateBounds struct {
	From          time.Time
	ToInclusive   time.Time
	ToExclusive   time.Time
	DaysInclusive int
}

type dailyMetric struct {
	Date                      time.Time
	PRCycleTimeMinutes        float64
	ReviewWaitMinutes         float64
	AverageReviewMinutes      float64
	AverageFilesChanged       float64
	AverageAdditions          float64
	AverageDeletions          float64
	DeploymentFrequency       float64
	ChangeFailureRate         float64
	ReviewCoverage            float64
	PRCount                   int64
	MergedPRCount             int64
	ReviewedPRCount           int64
	ReviewWaitSampleCount     int64
	ReviewTimeSampleCount     int64
	SuccessfulDeploymentCount int64
	FailedDeploymentCount     int64
}

func (d dailyMetric) toClickHouseRecord(repositoryID string, calculatedAt time.Time) metricsDailyRecord {
	return metricsDailyRecord{
		RepositoryID:              repositoryID,
		MetricDate:                formatDate(d.Date),
		PRCycleTimeMinutes:        d.PRCycleTimeMinutes,
		ReviewWaitMinutes:         d.ReviewWaitMinutes,
		AverageReviewMinutes:      d.AverageReviewMinutes,
		AverageFilesChanged:       d.AverageFilesChanged,
		AverageAdditions:          d.AverageAdditions,
		AverageDeletions:          d.AverageDeletions,
		DeploymentFrequency:       d.DeploymentFrequency,
		ChangeFailureRate:         d.ChangeFailureRate,
		ReviewCoverage:            d.ReviewCoverage,
		PRCount:                   d.PRCount,
		MergedPRCount:             d.MergedPRCount,
		ReviewedPRCount:           d.ReviewedPRCount,
		ReviewWaitSampleCount:     d.ReviewWaitSampleCount,
		ReviewTimeSampleCount:     d.ReviewTimeSampleCount,
		SuccessfulDeploymentCount: d.SuccessfulDeploymentCount,
		FailedDeploymentCount:     d.FailedDeploymentCount,
		CalculatedAt:              formatTimestamp(calculatedAt),
	}
}

func (s *Service) buildDailyMetrics(ctx context.Context, repositoryID pgtype.UUID, bounds dateBounds) ([]dailyMetric, error) {
	metricsByDay := make(map[string]*dailyMetric, bounds.DaysInclusive)
	for day := bounds.From; !day.After(bounds.ToInclusive); day = day.AddDate(0, 0, 1) {
		dayCopy := day
		metricsByDay[formatDate(dayCopy)] = &dailyMetric{Date: dayCopy}
	}

	prSizes, err := s.pg.Queries().AggregatePRSizeByDay(ctx, sqlcgen.AggregatePRSizeByDayParams{
		RepositoryID: repositoryID,
		CreatedAt:    toTimestamp(bounds.From),
		CreatedAt_2:  toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return nil, fmt.Errorf("aggregate pr size metrics: %w", err)
	}
	for _, row := range prSizes {
		day := row.MetricDate.Time.UTC()
		item := metricsByDay[formatDate(day)]
		item.PRCount = row.PrCount
		item.ReviewedPRCount = row.ReviewedPrCount
		item.AverageFilesChanged = row.AverageFilesChanged
		item.AverageAdditions = row.AverageAdditions
		item.AverageDeletions = row.AverageDeletions
	}

	prCycles, err := s.pg.Queries().AggregatePRCycleByDay(ctx, sqlcgen.AggregatePRCycleByDayParams{
		RepositoryID: repositoryID,
		MergedAt:     toTimestamp(bounds.From),
		MergedAt_2:   toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return nil, fmt.Errorf("aggregate pr cycle metrics: %w", err)
	}
	for _, row := range prCycles {
		day := row.MetricDate.Time.UTC()
		item := metricsByDay[formatDate(day)]
		item.MergedPRCount = row.MergedPrCount
		item.PRCycleTimeMinutes = row.PrCycleTimeMinutes
	}

	reviewMetrics, err := s.pg.Queries().AggregateReviewMetricsByDay(ctx, sqlcgen.AggregateReviewMetricsByDayParams{
		RepositoryID: repositoryID,
		CreatedAt:    toTimestamp(bounds.From),
		CreatedAt_2:  toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return nil, fmt.Errorf("aggregate review metrics: %w", err)
	}
	for _, row := range reviewMetrics {
		day := row.MetricDate.Time.UTC()
		item := metricsByDay[formatDate(day)]
		item.ReviewWaitSampleCount = row.ReviewWaitSampleCount
		item.ReviewWaitMinutes = row.AverageReviewWaitMinutes
		item.ReviewTimeSampleCount = row.ReviewTimeSampleCount
		item.AverageReviewMinutes = row.AverageReviewMinutes
	}

	deployments, err := s.pg.Queries().AggregateDeploymentsByDay(ctx, sqlcgen.AggregateDeploymentsByDayParams{
		RepositoryID: repositoryID,
		DeployedAt:   toTimestamp(bounds.From),
		DeployedAt_2: toTimestamp(bounds.ToExclusive),
	})
	if err != nil {
		return nil, fmt.Errorf("aggregate deployment metrics: %w", err)
	}
	for _, row := range deployments {
		day := row.MetricDate.Time.UTC()
		item := metricsByDay[formatDate(day)]
		item.SuccessfulDeploymentCount = row.SuccessfulDeploymentCount
		item.FailedDeploymentCount = row.FailedDeploymentCount
		item.DeploymentFrequency = float64(row.SuccessfulDeploymentCount)
		totalDeployments := row.SuccessfulDeploymentCount + row.FailedDeploymentCount
		if totalDeployments > 0 {
			item.ChangeFailureRate = float64(row.FailedDeploymentCount) / float64(totalDeployments)
		}
	}

	rows := make([]dailyMetric, 0, len(metricsByDay))
	for day := bounds.From; !day.After(bounds.ToInclusive); day = day.AddDate(0, 0, 1) {
		item := metricsByDay[formatDate(day)]
		if item.PRCount > 0 {
			item.ReviewCoverage = float64(item.ReviewedPRCount) / float64(item.PRCount)
		}
		rows = append(rows, *item)
	}

	return rows, nil
}
