package insights

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrRepositoryNotFound   = errors.New("repository not found")
	ErrInsightNotFound      = errors.New("insight not found")
)

type Repository struct {
	db    *postgres.DB
	rules RuleConfig
}

type repositoryRecord struct {
	ID       string
	Name     string
	FullName string
}

type statusRecord struct {
	InsightKey  string
	InsightType string
	Status      string
	ReviewedBy  *string
	ReviewedAt  *time.Time
	DismissedAt *time.Time
	ReopenedAt  *time.Time
}

type upsertStatusParams struct {
	OrganizationID string
	RepositoryID   *string
	InsightKey     string
	InsightType    string
	Status         string
	Evidence       map[string]any
	ReviewedBy     *string
	ReviewedAt     *time.Time
	DismissedAt    *time.Time
	ReopenedAt     *time.Time
	UpdatedAt      time.Time
}

func NewRepository(db *postgres.DB, rules ...RuleConfig) *Repository {
	cfg := DefaultRuleConfig()
	if len(rules) > 0 {
		cfg = normalizeRuleConfig(rules[0])
	}
	return &Repository{db: db, rules: cfg}
}

func (r *Repository) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	var exists bool
	err := r.db.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE id = $1)`, parseUUID(organizationID)).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check organization exists: %w", err)
	}
	if !exists {
		return ErrOrganizationNotFound
	}
	return nil
}

func (r *Repository) EnsureRepositoryInOrganization(ctx context.Context, organizationID string, repositoryID string) error {
	var exists bool
	err := r.db.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE id = $1 AND organization_id = $2)`, parseUUID(repositoryID), parseUUID(organizationID)).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check repository exists: %w", err)
	}
	if !exists {
		return ErrRepositoryNotFound
	}
	return nil
}

func (r *Repository) ListRepositories(ctx context.Context, organizationID string, repositoryID string) ([]repositoryRecord, error) {
	query := `
SELECT id::text, name, full_name
FROM repositories
WHERE organization_id = $1
  AND ($2::uuid IS NULL OR id = $2::uuid)
ORDER BY full_name ASC`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(organizationID), nullableUUID(repositoryID))
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	defer rows.Close()

	items := make([]repositoryRecord, 0)
	for rows.Next() {
		var item repositoryRecord
		if err := rows.Scan(&item.ID, &item.Name, &item.FullName); err != nil {
			return nil, fmt.Errorf("scan repository: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repositories: %w", err)
	}
	return items, nil
}

func (r *Repository) ListLargePullRequests(ctx context.Context, repositoryID string, from time.Time, to time.Time) ([]Insight, error) {
	query := `
SELECT number, title, additions, deletions, files_changed, created_at
FROM pull_requests
WHERE repository_id = $1
  AND created_at >= $2
  AND created_at < $3
  AND is_draft = FALSE
  AND (files_changed >= $4 OR additions + deletions >= $5)
ORDER BY created_at DESC`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(repositoryID), from, endExclusive(to), r.rules.LargePR.FilesThreshold, r.rules.LargePR.TotalChangesThreshold)
	if err != nil {
		return nil, fmt.Errorf("list large pull requests: %w", err)
	}
	defer rows.Close()

	var items []Insight
	for rows.Next() {
		var number int
		var title string
		var additions, deletions, filesChanged int
		var createdAt time.Time
		if err := rows.Scan(&number, &title, &additions, &deletions, &filesChanged, &createdAt); err != nil {
			return nil, fmt.Errorf("scan large pull request: %w", err)
		}

		totalChanges := additions + deletions
		severity := SeverityMedium
		if filesChanged >= r.rules.LargePR.HighSeverityFilesThreshold || totalChanges >= r.rules.LargePR.HighSeverityChangeThreshold {
			severity = SeverityHigh
		}
		items = append(items, Insight{
			InsightKey:   buildInsightKey(TypeLargePRDetection, repositoryID, fmt.Sprintf("pr-%d", number)),
			InsightType:  TypeLargePRDetection,
			Severity:     severity,
			Title:        fmt.Sprintf("Large PR #%d needs extra review attention", number),
			Summary:      fmt.Sprintf("PR #%d touched %d files with %d total line changes.", number, filesChanged, totalChanges),
			RepositoryID: repositoryID,
			DetectedAt:   createdAt.UTC(),
			Evidence: map[string]any{
				"entityKey":         fmt.Sprintf("pr-%d", number),
				"pullRequestNumber": number,
				"title":             title,
				"filesChanged":      filesChanged,
				"additions":         additions,
				"deletions":         deletions,
				"totalChanges":      totalChanges,
			},
		})
	}
	return items, rows.Err()
}

func (r *Repository) ListSlowReviews(ctx context.Context, repositoryID string, from time.Time, to time.Time) ([]Insight, error) {
	query := `
SELECT pr.number,
       pr.title,
       MIN(prr.review_requested_at) AS review_requested_at,
       MIN(COALESCE(prr.first_review_at, prr.review_submitted_at)) AS first_response_at
FROM pull_requests pr
INNER JOIN pull_request_reviews prr ON prr.pull_request_id = pr.id
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
  AND prr.review_requested_at IS NOT NULL
  AND COALESCE(prr.first_review_at, prr.review_submitted_at) IS NOT NULL
GROUP BY pr.number, pr.title
HAVING EXTRACT(EPOCH FROM MIN(COALESCE(prr.first_review_at, prr.review_submitted_at) - prr.review_requested_at)) / 3600.0 >= $4
ORDER BY review_requested_at DESC`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(repositoryID), from, endExclusive(to), r.rules.SlowReview.WaitHoursThreshold)
	if err != nil {
		return nil, fmt.Errorf("list slow reviews: %w", err)
	}
	defer rows.Close()

	var items []Insight
	for rows.Next() {
		var number int
		var title string
		var requestedAt, firstResponseAt time.Time
		if err := rows.Scan(&number, &title, &requestedAt, &firstResponseAt); err != nil {
			return nil, fmt.Errorf("scan slow review: %w", err)
		}
		waitHours := firstResponseAt.Sub(requestedAt).Hours()
		severity := SeverityMedium
		if waitHours >= r.rules.SlowReview.HighSeverityWaitHoursThreshold {
			severity = SeverityHigh
		}
		items = append(items, Insight{
			InsightKey:   buildInsightKey(TypeSlowReviewDetection, repositoryID, fmt.Sprintf("pr-%d", number)),
			InsightType:  TypeSlowReviewDetection,
			Severity:     severity,
			Title:        fmt.Sprintf("Slow review response on PR #%d", number),
			Summary:      fmt.Sprintf("PR #%d waited %.1f hours for first review response.", number, waitHours),
			RepositoryID: repositoryID,
			DetectedAt:   firstResponseAt.UTC(),
			Evidence: map[string]any{
				"entityKey":         fmt.Sprintf("pr-%d", number),
				"pullRequestNumber": number,
				"title":             title,
				"reviewRequestedAt": requestedAt.UTC(),
				"firstResponseAt":   firstResponseAt.UTC(),
				"waitHours":         waitHours,
			},
		})
	}
	return items, rows.Err()
}

func (r *Repository) ListHotspots(ctx context.Context, repositoryID string, from time.Time, to time.Time) ([]Insight, error) {
	query := `
SELECT fc.file_path,
       SUM(fc.additions) AS additions,
       SUM(fc.deletions) AS deletions,
       SUM(fc.commit_count) AS commit_count,
       SUM(fc.additions + fc.deletions + (fc.commit_count * 5)) AS hotspot_score,
       MAX(pr.created_at) AS detected_at
FROM file_changes fc
INNER JOIN pull_requests pr ON pr.id = fc.pull_request_id
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
GROUP BY fc.file_path
HAVING SUM(fc.additions + fc.deletions + (fc.commit_count * 5)) >= $4
ORDER BY hotspot_score DESC
LIMIT $5`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(repositoryID), from, endExclusive(to), r.rules.Hotspot.ScoreThreshold, r.rules.Hotspot.TopFilesLimit)
	if err != nil {
		return nil, fmt.Errorf("list hotspots: %w", err)
	}
	defer rows.Close()

	var items []Insight
	for rows.Next() {
		var filePath string
		var additions, deletions, commitCount int
		var hotspotScore int
		var detectedAt time.Time
		if err := rows.Scan(&filePath, &additions, &deletions, &commitCount, &hotspotScore, &detectedAt); err != nil {
			return nil, fmt.Errorf("scan hotspot: %w", err)
		}
		severity := SeverityMedium
		if hotspotScore >= r.rules.Hotspot.HighSeverityScoreThreshold {
			severity = SeverityHigh
		}
		items = append(items, Insight{
			InsightKey:   buildInsightKey(TypeHotspotDetection, repositoryID, hashKeyPart(filePath)),
			InsightType:  TypeHotspotDetection,
			Severity:     severity,
			Title:        "Hotspot file needs stabilizing",
			Summary:      fmt.Sprintf("%s changed heavily across recent pull requests.", filePath),
			RepositoryID: repositoryID,
			DetectedAt:   detectedAt.UTC(),
			Evidence: map[string]any{
				"entityKey":    strings.ToLower(filePath),
				"filePath":     filePath,
				"hotspotScore": hotspotScore,
				"additions":    additions,
				"deletions":    deletions,
				"commitCount":  commitCount,
			},
		})
	}
	return items, rows.Err()
}

func (r *Repository) ListDeploymentFailureTrends(ctx context.Context, repositoryID string, from time.Time, to time.Time) ([]Insight, error) {
	query := `
SELECT environment,
       COUNT(*) AS total_count,
       COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
       MAX(deployed_at) AS detected_at
FROM deployments
WHERE repository_id = $1
  AND deployed_at >= $2
  AND deployed_at < $3
GROUP BY environment
HAVING COUNT(*) >= $4
   AND (COUNT(*) FILTER (WHERE status = 'failed'))::double precision / COUNT(*)::double precision >= $5
ORDER BY detected_at DESC`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(repositoryID), from, endExclusive(to), r.rules.DeploymentFailure.MinimumDeployments, r.rules.DeploymentFailure.FailureRateThreshold)
	if err != nil {
		return nil, fmt.Errorf("list deployment failure trends: %w", err)
	}
	defer rows.Close()

	var items []Insight
	for rows.Next() {
		var environment string
		var totalCount, failedCount int
		var detectedAt time.Time
		if err := rows.Scan(&environment, &totalCount, &failedCount, &detectedAt); err != nil {
			return nil, fmt.Errorf("scan deployment failure trend: %w", err)
		}
		failureRate := float64(failedCount) / float64(totalCount)
		severity := SeverityMedium
		if failureRate >= r.rules.DeploymentFailure.HighSeverityFailureRate {
			severity = SeverityHigh
		}
		items = append(items, Insight{
			InsightKey:   buildInsightKey(TypeDeploymentFailureTrend, repositoryID, hashKeyPart(environment)),
			InsightType:  TypeDeploymentFailureTrend,
			Severity:     severity,
			Title:        "Deployment failures are trending up",
			Summary:      fmt.Sprintf("%s has %.0f%% failed deployments in the selected window.", environment, failureRate*100),
			RepositoryID: repositoryID,
			DetectedAt:   detectedAt.UTC(),
			Evidence: map[string]any{
				"entityKey":   strings.ToLower(environment),
				"environment": environment,
				"totalCount":  totalCount,
				"failedCount": failedCount,
				"failureRate": failureRate,
			},
		})
	}
	return items, rows.Err()
}

func (r *Repository) ListReviewConcentration(ctx context.Context, repositoryID string, from time.Time, to time.Time) ([]Insight, error) {
	query := `
SELECT reviewer,
       COUNT(*) AS review_count,
       MAX(COALESCE(review_submitted_at, first_review_at, review_requested_at)) AS detected_at
FROM pull_request_reviews prr
INNER JOIN pull_requests pr ON pr.id = prr.pull_request_id
WHERE pr.repository_id = $1
  AND pr.created_at >= $2
  AND pr.created_at < $3
GROUP BY reviewer`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(repositoryID), from, endExclusive(to))
	if err != nil {
		return nil, fmt.Errorf("list review concentration: %w", err)
	}
	defer rows.Close()

	type reviewAggregate struct {
		reviewer    string
		reviewCount int
		detectedAt  time.Time
	}

	aggregates := make([]reviewAggregate, 0)
	totalReviews := 0
	for rows.Next() {
		var item reviewAggregate
		if err := rows.Scan(&item.reviewer, &item.reviewCount, &item.detectedAt); err != nil {
			return nil, fmt.Errorf("scan review concentration: %w", err)
		}
		totalReviews += item.reviewCount
		aggregates = append(aggregates, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var items []Insight
	for _, item := range aggregates {
		if totalReviews == 0 {
			continue
		}
		share := float64(item.reviewCount) / float64(totalReviews)
		if item.reviewCount < r.rules.ReviewConcentration.MinimumReviewCount || share < r.rules.ReviewConcentration.ShareThreshold {
			continue
		}
		severity := SeverityMedium
		if share >= r.rules.ReviewConcentration.HighSeverityShareThreshold {
			severity = SeverityHigh
		}
		items = append(items, Insight{
			InsightKey:   buildInsightKey(TypeReviewConcentration, repositoryID, hashKeyPart(item.reviewer)),
			InsightType:  TypeReviewConcentration,
			Severity:     severity,
			Title:        "Review ownership is concentrated",
			Summary:      fmt.Sprintf("%s handled %.0f%% of reviews in the selected window.", item.reviewer, share*100),
			RepositoryID: repositoryID,
			DetectedAt:   item.detectedAt.UTC(),
			Evidence: map[string]any{
				"entityKey":    strings.ToLower(item.reviewer),
				"reviewer":     item.reviewer,
				"reviewCount":  item.reviewCount,
				"totalReviews": totalReviews,
				"share":        share,
			},
		})
	}
	return items, nil
}

func (r *Repository) ListBottlenecks(ctx context.Context, repositoryID string, from time.Time, to time.Time) ([]Insight, error) {
	query := `
SELECT COUNT(*) FILTER (WHERE merged_at IS NOT NULL) AS merged_count,
       COALESCE(AVG(EXTRACT(EPOCH FROM (merged_at - created_at)) / 3600.0) FILTER (WHERE merged_at IS NOT NULL), 0) AS avg_cycle_hours,
       COUNT(*) FILTER (WHERE state = 'open' AND created_at < NOW() - ($4 * interval '1 day')) AS stale_open_count,
       MAX(COALESCE(merged_at, created_at)) AS detected_at
FROM pull_requests
WHERE repository_id = $1
  AND created_at >= $2
  AND created_at < $3
  AND is_draft = FALSE`

	var mergedCount, staleOpenCount int
	var avgCycleHours float64
	var detectedAt pgtype.Timestamptz
	if err := r.db.Pool().QueryRow(ctx, query, parseUUID(repositoryID), from, endExclusive(to), r.rules.Bottleneck.StaleOpenAgeDays).Scan(&mergedCount, &avgCycleHours, &staleOpenCount, &detectedAt); err != nil {
		return nil, fmt.Errorf("list bottlenecks: %w", err)
	}

	if mergedCount < r.rules.Bottleneck.MinimumMergedCount && staleOpenCount == 0 {
		return nil, nil
	}
	if avgCycleHours < r.rules.Bottleneck.AverageCycleHoursThreshold && staleOpenCount < r.rules.Bottleneck.StaleOpenCountThreshold {
		return nil, nil
	}

	severity := SeverityMedium
	if avgCycleHours >= r.rules.Bottleneck.HighSeverityCycleHoursThreshold || staleOpenCount >= r.rules.Bottleneck.HighSeverityStaleOpenThreshold {
		severity = SeverityHigh
	}

	detected := time.Now().UTC()
	if detectedAt.Valid {
		detected = detectedAt.Time.UTC()
	}

	item := Insight{
		InsightKey:   buildInsightKey(TypeBottleneckDetection, repositoryID, "summary"),
		InsightType:  TypeBottleneckDetection,
		Severity:     severity,
		Title:        "Flow bottleneck detected",
		Summary:      fmt.Sprintf("Merged PRs averaged %.1f hours cycle time with %d stale open PRs.", avgCycleHours, staleOpenCount),
		RepositoryID: repositoryID,
		DetectedAt:   detected,
		Evidence: map[string]any{
			"entityKey":         "summary",
			"mergedCount":       mergedCount,
			"averageCycleHours": avgCycleHours,
			"staleOpenCount":    staleOpenCount,
		},
	}
	return []Insight{item}, nil
}

func (r *Repository) ListStatusesByKeys(ctx context.Context, organizationID string, keys []string) (map[string]statusRecord, error) {
	if len(keys) == 0 {
		return map[string]statusRecord{}, nil
	}

	query := `
SELECT insight_key,
       insight_type,
       status,
       reviewed_by::text,
       reviewed_at,
       dismissed_at,
       reopened_at
FROM insight_statuses
WHERE organization_id = $1
  AND insight_key = ANY($2)`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(organizationID), keys)
	if err != nil {
		return nil, fmt.Errorf("list insight statuses: %w", err)
	}
	defer rows.Close()

	result := make(map[string]statusRecord, len(keys))
	for rows.Next() {
		var item statusRecord
		var reviewedBy pgtype.Text
		var reviewedAt, dismissedAt, reopenedAt pgtype.Timestamptz
		if err := rows.Scan(&item.InsightKey, &item.InsightType, &item.Status, &reviewedBy, &reviewedAt, &dismissedAt, &reopenedAt); err != nil {
			return nil, fmt.Errorf("scan insight status: %w", err)
		}
		item.ReviewedBy = optionalText(reviewedBy)
		item.ReviewedAt = optionalTime(reviewedAt)
		item.DismissedAt = optionalTime(dismissedAt)
		item.ReopenedAt = optionalTime(reopenedAt)
		result[item.InsightKey] = item
	}
	return result, rows.Err()
}

func (r *Repository) GetStatusByKey(ctx context.Context, organizationID string, insightKey string) (statusRecord, error) {
	query := `
SELECT insight_key,
       insight_type,
       status,
       reviewed_by::text,
       reviewed_at,
       dismissed_at,
       reopened_at
FROM insight_statuses
WHERE organization_id = $1
  AND insight_key = $2`

	var item statusRecord
	var reviewedBy pgtype.Text
	var reviewedAt, dismissedAt, reopenedAt pgtype.Timestamptz
	err := r.db.Pool().QueryRow(ctx, query, parseUUID(organizationID), insightKey).Scan(
		&item.InsightKey,
		&item.InsightType,
		&item.Status,
		&reviewedBy,
		&reviewedAt,
		&dismissedAt,
		&reopenedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return statusRecord{}, ErrInsightNotFound
		}
		return statusRecord{}, fmt.Errorf("get insight status: %w", err)
	}
	item.ReviewedBy = optionalText(reviewedBy)
	item.ReviewedAt = optionalTime(reviewedAt)
	item.DismissedAt = optionalTime(dismissedAt)
	item.ReopenedAt = optionalTime(reopenedAt)
	return item, nil
}

func (r *Repository) UpsertStatus(ctx context.Context, params upsertStatusParams) (StatusResult, error) {
	evidenceBytes, err := json.Marshal(params.Evidence)
	if err != nil {
		return StatusResult{}, fmt.Errorf("marshal insight evidence: %w", err)
	}

	query := `
INSERT INTO insight_statuses (
    id,
    organization_id,
    repository_id,
    insight_key,
    insight_type,
    status,
    evidence_json,
    reviewed_by,
    reviewed_at,
    dismissed_at,
    reopened_at,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $12)
ON CONFLICT (organization_id, insight_key) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    insight_type = EXCLUDED.insight_type,
    status = EXCLUDED.status,
    evidence_json = EXCLUDED.evidence_json,
    reviewed_by = EXCLUDED.reviewed_by,
    reviewed_at = EXCLUDED.reviewed_at,
    dismissed_at = EXCLUDED.dismissed_at,
    reopened_at = EXCLUDED.reopened_at,
    updated_at = EXCLUDED.updated_at
RETURNING insight_key,
          insight_type,
          status,
          reviewed_by::text,
          reviewed_at,
          dismissed_at,
          reopened_at,
          updated_at`

	var result StatusResult
	var reviewedBy pgtype.Text
	var reviewedAt, dismissedAt, reopenedAt, updatedAt pgtype.Timestamptz
	err = r.db.Pool().QueryRow(ctx, query,
		newUUID(),
		parseUUID(params.OrganizationID),
		nullableUUIDPtr(params.RepositoryID),
		params.InsightKey,
		params.InsightType,
		params.Status,
		string(evidenceBytes),
		nullableUUIDString(params.ReviewedBy),
		nullableTime(params.ReviewedAt),
		nullableTime(params.DismissedAt),
		nullableTime(params.ReopenedAt),
		params.UpdatedAt.UTC(),
	).Scan(
		&result.InsightKey,
		&result.InsightType,
		&result.Status,
		&reviewedBy,
		&reviewedAt,
		&dismissedAt,
		&reopenedAt,
		&updatedAt,
	)
	if err != nil {
		return StatusResult{}, fmt.Errorf("upsert insight status: %w", err)
	}
	result.ReviewedBy = optionalText(reviewedBy)
	result.ReviewedAt = optionalTime(reviewedAt)
	result.DismissedAt = optionalTime(dismissedAt)
	result.ReopenedAt = optionalTime(reopenedAt)
	if updatedAt.Valid {
		result.UpdatedAt = updatedAt.Time.UTC()
	}
	return result, nil
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	id := parseUUID(value)
	if !id.Valid {
		return nil
	}
	return id
}

func nullableUUIDPtr(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	id := parseUUID(*value)
	if !id.Valid {
		return nil
	}
	return id
}

func nullableUUIDString(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	id := parseUUID(*value)
	if !id.Valid {
		return nil
	}
	return id
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func newUUID() pgtype.UUID {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	return pgtype.UUID{Bytes: bytes, Valid: true}
}
