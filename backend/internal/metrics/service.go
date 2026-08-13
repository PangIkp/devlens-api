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

const clickhouseInsertBatchSize = 500

type Service struct {
	pg    *postgres.DB
	ch    *clickhouse.DB
	rules RuleConfig
}

type RuleConfig struct {
	DefaultDayType         string
	HotspotCommitWeight    float64
	HotspotAdditionsWeight float64
	HotspotDeletionsWeight float64
}

func DefaultRuleConfig() RuleConfig {
	return RuleConfig{
		DefaultDayType:         DayTypeCalendar,
		HotspotCommitWeight:    1,
		HotspotAdditionsWeight: 1,
		HotspotDeletionsWeight: 1,
	}
}

func normalizeRuleConfig(cfg RuleConfig) RuleConfig {
	defaults := DefaultRuleConfig()
	if cfg.DefaultDayType != DayTypeBusiness && cfg.DefaultDayType != DayTypeCalendar {
		cfg.DefaultDayType = defaults.DefaultDayType
	}
	if cfg.HotspotCommitWeight < 0 {
		cfg.HotspotCommitWeight = defaults.HotspotCommitWeight
	}
	if cfg.HotspotAdditionsWeight < 0 {
		cfg.HotspotAdditionsWeight = defaults.HotspotAdditionsWeight
	}
	if cfg.HotspotDeletionsWeight < 0 {
		cfg.HotspotDeletionsWeight = defaults.HotspotDeletionsWeight
	}
	if cfg.HotspotCommitWeight == 0 && cfg.HotspotAdditionsWeight == 0 && cfg.HotspotDeletionsWeight == 0 {
		cfg.HotspotCommitWeight = defaults.HotspotCommitWeight
		cfg.HotspotAdditionsWeight = defaults.HotspotAdditionsWeight
		cfg.HotspotDeletionsWeight = defaults.HotspotDeletionsWeight
	}
	return cfg
}

func NewService(pg *postgres.DB, ch *clickhouse.DB, rules ...RuleConfig) *Service {
	cfg := DefaultRuleConfig()
	if len(rules) > 0 {
		cfg = normalizeRuleConfig(rules[0])
	}
	return &Service{pg: pg, ch: ch, rules: cfg}
}

func (s *Service) CalculateRepositoryMetrics(ctx context.Context, repositoryID string, req CalculationRequest) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	req = normalizeCalculationRequest(req)
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
		payload = append(payload, row.toClickHouseRecord(repositoryID, calculatedAt, req.MetricVersion))
	}

	if err := s.ch.InsertJSONEachRowBatched(ctx, "INSERT INTO metrics_daily", payload, clickhouseInsertBatchSize); err != nil {
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
	dayType, err := normalizeDayType(params.DayType)
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

	summary := aggregateSummary(repositoryID, bounds, dayType, rows)
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
	dayType, err := normalizeDayType(params.DayType)
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

	return aggregatePullRequestMetrics(bounds, interval, dayType, rows), nil
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
	dayType, err := normalizeDayType(params.DayType)
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

	return aggregateReviewMetrics(bounds, interval, dayType, rows), nil
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
	dayType, err := normalizeDayType(params.DayType)
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
		return aggregateDeploymentMetricsFromRaw(bounds, interval, dayType, deployments), nil
	}

	rows, err := s.listMetricsDaily(ctx, repositoryID, bounds)
	if err != nil {
		return DeploymentMetrics{}, err
	}

	return aggregateDeploymentMetrics(bounds, interval, dayType, rows), nil
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

	files := aggregateHotspots(rows, s.rules)
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

func (s *Service) GetRepositoryMetrics(ctx context.Context, repositoryID string, params DeploymentQueryParams) (RepositoryMetrics, error) {
	summary, err := s.GetDashboardSummary(ctx, repositoryID, params.QueryParams)
	if err != nil {
		return RepositoryMetrics{}, err
	}

	pullRequests, err := s.GetPullRequestMetrics(ctx, repositoryID, params.QueryParams)
	if err != nil {
		return RepositoryMetrics{}, err
	}

	reviews, err := s.GetReviewMetrics(ctx, repositoryID, params.QueryParams)
	if err != nil {
		return RepositoryMetrics{}, err
	}

	deployments, err := s.GetDeploymentMetrics(ctx, repositoryID, params)
	if err != nil {
		return RepositoryMetrics{}, err
	}

	hotspots, err := s.GetHotspots(ctx, repositoryID, HotspotQueryParams{
		From:      params.From,
		To:        params.To,
		Page:      1,
		PageSize:  10,
		SortOrder: "desc",
	})
	if err != nil {
		return RepositoryMetrics{}, err
	}

	interval := params.Interval
	if strings.TrimSpace(interval) == "" {
		interval = IntervalDay
	}

	return RepositoryMetrics{
		MetricVersion: CurrentMetricVersion,
		DayType:       s.normalizeDayTypeOrDefault(params.DayType),
		RepositoryID:  repositoryID,
		From:          params.From.UTC().Format("2006-01-02"),
		To:            params.To.UTC().Format("2006-01-02"),
		Interval:      interval,
		Summary:       summary,
		PullRequests:  pullRequests,
		Reviews:       reviews,
		Deployments:   deployments,
		Hotspots:      hotspots.Items,
	}, nil
}

func (s *Service) GetWorkloadDistribution(ctx context.Context, repositoryID string, params QueryParams) (WorkloadDistribution, error) {
	if s.pg == nil {
		return WorkloadDistribution{}, fmt.Errorf("metrics postgres dependency is not configured")
	}

	bounds, err := normalizeBounds(params.From, params.To)
	if err != nil {
		return WorkloadDistribution{}, err
	}

	if _, err := s.ensureRepositoryExists(ctx, repositoryID); err != nil {
		return WorkloadDistribution{}, err
	}

	contributors, err := s.listContributorDistribution(ctx, repositoryID, bounds)
	if err != nil {
		return WorkloadDistribution{}, err
	}

	reviewers, err := s.listReviewerDistribution(ctx, repositoryID, bounds)
	if err != nil {
		return WorkloadDistribution{}, err
	}

	totalPullRequests := 0
	topContributorShare := 0.0
	for _, item := range contributors {
		totalPullRequests += item.PullRequestCount
		if item.Share > topContributorShare {
			topContributorShare = item.Share
		}
	}

	totalReviews := 0
	topReviewerShare := 0.0
	for _, item := range reviewers {
		totalReviews += item.ReviewCount
		if item.Share > topReviewerShare {
			topReviewerShare = item.Share
		}
	}

	return WorkloadDistribution{
		Summary: WorkloadDistributionSummary{
			RepositoryID:        repositoryID,
			From:                bounds.From.Format("2006-01-02"),
			To:                  bounds.ToInclusive.Format("2006-01-02"),
			TotalPullRequests:   totalPullRequests,
			TotalReviews:        totalReviews,
			TopContributorShare: topContributorShare,
			TopReviewerShare:    topReviewerShare,
		},
		Contributors: contributors,
		Reviewers:    reviewers,
	}, nil
}

func normalizeCalculationRequest(req CalculationRequest) CalculationRequest {
	if req.MetricVersion < 1 {
		req.MetricVersion = CurrentMetricVersion
	}
	return req
}

func (s *Service) GetReviewQueue(ctx context.Context, repositoryID string, params HotspotQueryParams) (ReviewQueueResult, error) {
	if s.pg == nil {
		return ReviewQueueResult{}, fmt.Errorf("metrics postgres dependency is not configured")
	}
	if _, err := s.ensureRepositoryExists(ctx, repositoryID); err != nil {
		return ReviewQueueResult{}, err
	}
	if params.Page < 1 || params.PageSize < 1 {
		return ReviewQueueResult{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "page", Message: "must be greater than or equal to 1"}},
		}
	}

	toExclusive := params.To.UTC().Add(24 * time.Hour)
	query := `
WITH review_queue AS (
    SELECT pr.id::text AS pull_request_id,
           pr.number,
           pr.title,
           pr.author,
           MIN(prr.review_requested_at) AS review_requested_at
    FROM pull_requests pr
    LEFT JOIN pull_request_reviews prr ON prr.pull_request_id = pr.id
    WHERE pr.repository_id = $1
      AND pr.state = 'open'
      AND pr.created_at >= $2
      AND pr.created_at < $3
      AND pr.is_draft = FALSE
    GROUP BY pr.id, pr.number, pr.title, pr.author
)
SELECT pull_request_id,
       number,
       title,
       author,
       review_requested_at,
       CASE
           WHEN review_requested_at IS NULL THEN EXTRACT(EPOCH FROM (NOW() - $2)) / 60.0
           ELSE EXTRACT(EPOCH FROM (NOW() - review_requested_at)) / 60.0
       END AS waiting_minutes
FROM review_queue
ORDER BY review_requested_at ASC NULLS FIRST, number ASC
LIMIT $4 OFFSET $5`

	rows, err := s.pg.Pool().Query(ctx, query, parseUUID(repositoryID), params.From.UTC(), toExclusive, params.PageSize, (params.Page-1)*params.PageSize)
	if err != nil {
		return ReviewQueueResult{}, fmt.Errorf("list review queue: %w", err)
	}
	defer rows.Close()

	result := ReviewQueueResult{Items: make([]ReviewQueueItem, 0)}
	for rows.Next() {
		var item ReviewQueueItem
		var requestedAt pgtype.Timestamptz
		if err := rows.Scan(&item.PullRequestID, &item.Number, &item.Title, &item.Author, &requestedAt, &item.WaitingMinutes); err != nil {
			return ReviewQueueResult{}, fmt.Errorf("scan review queue item: %w", err)
		}
		item.ReviewRequestedAt = optionalMetricTime(requestedAt)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ReviewQueueResult{}, fmt.Errorf("iterate review queue: %w", err)
	}

	countQuery := `
SELECT COUNT(*)
FROM pull_requests
WHERE repository_id = $1
  AND state = 'open'
  AND created_at >= $2
  AND created_at < $3
  AND is_draft = FALSE`
	if err := s.pg.Pool().QueryRow(ctx, countQuery, parseUUID(repositoryID), params.From.UTC(), toExclusive).Scan(&result.TotalItems); err != nil {
		return ReviewQueueResult{}, fmt.Errorf("count review queue: %w", err)
	}
	return result, nil
}

func optionalMetricTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}

func (s *Service) listContributorDistribution(ctx context.Context, repositoryID string, bounds dateBounds) ([]ContributorDistributionItem, error) {
	toExclusive := bounds.ToInclusive.Add(24 * time.Hour)
	query := `
SELECT pr.author,
       COUNT(*)::bigint AS pull_request_count
FROM pull_requests pr
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
  AND pr.is_draft = FALSE
  AND pr.author IS NOT NULL
  AND btrim(pr.author) <> ''
  AND lower(btrim(pr.author)) NOT LIKE '%[bot]'
  AND lower(btrim(pr.author)) NOT LIKE 'dependabot%'
  AND lower(btrim(pr.author)) NOT LIKE '%-bot'
  AND lower(btrim(pr.author)) <> 'github-actions'
  AND lower(btrim(pr.author)) <> 'web-flow'
GROUP BY pr.author
ORDER BY pull_request_count DESC, pr.author ASC`

	rows, err := s.pg.Pool().Query(ctx, query, parseUUID(repositoryID), bounds.From.UTC(), toExclusive)
	if err != nil {
		return nil, fmt.Errorf("list contributor distribution: %w", err)
	}
	defer rows.Close()

	type rawItem struct {
		author string
		count  int
	}
	raw := make([]rawItem, 0)
	total := 0
	for rows.Next() {
		var item rawItem
		var count int64
		if err := rows.Scan(&item.author, &count); err != nil {
			return nil, fmt.Errorf("scan contributor distribution: %w", err)
		}
		item.count = int(count)
		total += item.count
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contributor distribution: %w", err)
	}

	items := make([]ContributorDistributionItem, 0, len(raw))
	for _, item := range raw {
		share := 0.0
		if total > 0 {
			share = float64(item.count) / float64(total)
		}
		items = append(items, ContributorDistributionItem{
			Author:           item.author,
			PullRequestCount: item.count,
			Share:            share,
		})
	}
	return items, nil
}

func (s *Service) listReviewerDistribution(ctx context.Context, repositoryID string, bounds dateBounds) ([]ReviewerDistributionItem, error) {
	toExclusive := bounds.ToInclusive.Add(24 * time.Hour)
	query := `
SELECT prr.reviewer,
       COUNT(*)::bigint AS review_count,
       COUNT(DISTINCT prr.pull_request_id)::bigint AS reviewed_pull_request_count
FROM pull_request_reviews prr
INNER JOIN pull_requests pr ON pr.id = prr.pull_request_id
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
  AND pr.is_draft = FALSE
  AND prr.reviewer IS NOT NULL
  AND btrim(prr.reviewer) <> ''
  AND lower(btrim(prr.reviewer)) NOT LIKE '%[bot]'
  AND lower(btrim(prr.reviewer)) NOT LIKE 'dependabot%'
  AND lower(btrim(prr.reviewer)) NOT LIKE '%-bot'
  AND lower(btrim(prr.reviewer)) <> 'github-actions'
  AND lower(btrim(prr.reviewer)) <> 'web-flow'
GROUP BY prr.reviewer
ORDER BY review_count DESC, prr.reviewer ASC`

	rows, err := s.pg.Pool().Query(ctx, query, parseUUID(repositoryID), bounds.From.UTC(), toExclusive)
	if err != nil {
		return nil, fmt.Errorf("list reviewer distribution: %w", err)
	}
	defer rows.Close()

	type rawItem struct {
		reviewer                 string
		reviewCount              int
		reviewedPullRequestCount int
	}
	raw := make([]rawItem, 0)
	total := 0
	for rows.Next() {
		var item rawItem
		var reviewCount, reviewedPullRequestCount int64
		if err := rows.Scan(&item.reviewer, &reviewCount, &reviewedPullRequestCount); err != nil {
			return nil, fmt.Errorf("scan reviewer distribution: %w", err)
		}
		item.reviewCount = int(reviewCount)
		item.reviewedPullRequestCount = int(reviewedPullRequestCount)
		total += item.reviewCount
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reviewer distribution: %w", err)
	}

	items := make([]ReviewerDistributionItem, 0, len(raw))
	for _, item := range raw {
		share := 0.0
		if total > 0 {
			share = float64(item.reviewCount) / float64(total)
		}
		items = append(items, ReviewerDistributionItem{
			Reviewer:                 item.reviewer,
			ReviewCount:              item.reviewCount,
			ReviewedPullRequestCount: item.reviewedPullRequestCount,
			Share:                    share,
		})
	}
	return items, nil
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

func normalizeDayType(value string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return DayTypeCalendar, nil
	}

	switch trimmed {
	case DayTypeCalendar, DayTypeBusiness:
		return trimmed, nil
	default:
		return "", &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "dayType", Message: "must be one of calendar, business"}},
		}
	}
}

func (s *Service) normalizeDayTypeOrDefault(value string) string {
	dayType, err := normalizeDayType(value)
	if err != nil {
		return s.rules.DefaultDayType
	}
	return dayType
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

func (b dateBounds) dayCount(dayType string) int {
	switch dayType {
	case DayTypeBusiness:
		count := 0
		for day := b.From; !day.After(b.ToInclusive); day = day.AddDate(0, 0, 1) {
			weekday := day.Weekday()
			if weekday != time.Saturday && weekday != time.Sunday {
				count++
			}
		}
		return count
	default:
		return b.DaysInclusive
	}
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

func (d dailyMetric) toClickHouseRecord(repositoryID string, calculatedAt time.Time, metricVersion int) metricsDailyRecord {
	return metricsDailyRecord{
		MetricVersion:             int64(metricVersion),
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
