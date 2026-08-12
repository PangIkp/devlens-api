package insights

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type store interface {
	EnsureOrganizationExists(context.Context, string) error
	EnsureRepositoryInOrganization(context.Context, string, string) error
	ListRepositories(context.Context, string, string) ([]repositoryRecord, error)
	ListLargePullRequests(context.Context, string, time.Time, time.Time) ([]Insight, error)
	ListSlowReviews(context.Context, string, time.Time, time.Time) ([]Insight, error)
	ListHotspots(context.Context, string, time.Time, time.Time) ([]Insight, error)
	ListDeploymentFailureTrends(context.Context, string, time.Time, time.Time) ([]Insight, error)
	ListReviewConcentration(context.Context, string, time.Time, time.Time) ([]Insight, error)
	ListBottlenecks(context.Context, string, time.Time, time.Time) ([]Insight, error)
	ListStatusesByKeys(context.Context, string, []string) (map[string]statusRecord, error)
	GetStatusByKey(context.Context, string, string) (statusRecord, error)
	UpsertStatus(context.Context, upsertStatusParams) (StatusResult, error)
}

type Service struct {
	repository store
	now        func() time.Time
}

func NewService(repository store) *Service {
	return &Service{
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) List(ctx context.Context, params ListParams) (ListResult, error) {
	if err := validateListParams(params); err != nil {
		return ListResult{}, err
	}
	if err := s.repository.EnsureOrganizationExists(ctx, params.OrganizationID); err != nil {
		return ListResult{}, err
	}
	if params.RepositoryID != "" {
		if err := s.repository.EnsureRepositoryInOrganization(ctx, params.OrganizationID, params.RepositoryID); err != nil {
			return ListResult{}, err
		}
	}

	repositories, err := s.repository.ListRepositories(ctx, params.OrganizationID, params.RepositoryID)
	if err != nil {
		return ListResult{}, err
	}

	allInsights := make([]Insight, 0)
	for _, repo := range repositories {
		items, err := s.generateRepositoryInsights(ctx, repo, params.From, params.To)
		if err != nil {
			return ListResult{}, err
		}
		for i := range items {
			items[i].OrganizationID = params.OrganizationID
			items[i].RepositoryName = repo.FullName
			items[i].Status = StatusOpen
		}
		allInsights = append(allInsights, items...)
	}

	if len(allInsights) == 0 {
		return ListResult{Items: []Insight{}, TotalItems: 0}, nil
	}

	statuses, err := s.repository.ListStatusesByKeys(ctx, params.OrganizationID, insightKeys(allInsights))
	if err != nil {
		return ListResult{}, err
	}

	for i := range allInsights {
		if status, ok := statuses[allInsights[i].InsightKey]; ok {
			applyStatus(&allInsights[i], status)
			if shouldReopen(status, allInsights[i].DetectedAt) {
				result, reopenErr := s.repository.UpsertStatus(ctx, upsertStatusParams{
					OrganizationID: params.OrganizationID,
					RepositoryID:   optionalString(allInsights[i].RepositoryID),
					InsightKey:     allInsights[i].InsightKey,
					InsightType:    allInsights[i].InsightType,
					Status:         StatusOpen,
					Evidence:       allInsights[i].Evidence,
					ReviewedBy:     status.ReviewedBy,
					ReviewedAt:     status.ReviewedAt,
					DismissedAt:    nil,
					ReopenedAt:     timePtr(s.now()),
					UpdatedAt:      s.now(),
				})
				if reopenErr != nil {
					return ListResult{}, reopenErr
				}
				allInsights[i].Status = result.Status
				allInsights[i].ReviewedBy = result.ReviewedBy
				allInsights[i].ReviewedAt = result.ReviewedAt
				allInsights[i].DismissedAt = result.DismissedAt
				allInsights[i].ReopenedAt = result.ReopenedAt
			}
		}
	}

	filtered := filterInsights(allInsights, params.Type, params.Status)
	sort.Slice(filtered, func(i, j int) bool {
		if severityRank(filtered[i].Severity) == severityRank(filtered[j].Severity) {
			return filtered[i].DetectedAt.After(filtered[j].DetectedAt)
		}
		return severityRank(filtered[i].Severity) > severityRank(filtered[j].Severity)
	})

	totalItems := len(filtered)
	start := (params.Page - 1) * params.PageSize
	if start >= totalItems {
		return ListResult{Items: []Insight{}, TotalItems: totalItems}, nil
	}
	end := start + params.PageSize
	if end > totalItems {
		end = totalItems
	}
	return ListResult{Items: filtered[start:end], TotalItems: totalItems}, nil
}

func (s *Service) Review(ctx context.Context, organizationID string, insightKey string, req ReviewRequest) (StatusResult, error) {
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return StatusResult{}, err
	}
	now := s.now()
	repositoryID, insightType, err := parseInsightKey(insightKey)
	if err != nil {
		return StatusResult{}, err
	}
	reviewedBy := trimOptionalString(req.ReviewedBy)
	return s.repository.UpsertStatus(ctx, upsertStatusParams{
		OrganizationID: organizationID,
		RepositoryID:   optionalString(repositoryID),
		InsightKey:     insightKey,
		InsightType:    insightType,
		Status:         StatusReviewed,
		ReviewedBy:     reviewedBy,
		ReviewedAt:     &now,
		UpdatedAt:      now,
	})
}

func (s *Service) Dismiss(ctx context.Context, organizationID string, insightKey string) (StatusResult, error) {
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return StatusResult{}, err
	}
	now := s.now()
	repositoryID, insightType, err := parseInsightKey(insightKey)
	if err != nil {
		return StatusResult{}, err
	}
	return s.repository.UpsertStatus(ctx, upsertStatusParams{
		OrganizationID: organizationID,
		RepositoryID:   optionalString(repositoryID),
		InsightKey:     insightKey,
		InsightType:    insightType,
		Status:         StatusDismissed,
		DismissedAt:    &now,
		UpdatedAt:      now,
	})
}

func (s *Service) Reopen(ctx context.Context, organizationID string, insightKey string) (StatusResult, error) {
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return StatusResult{}, err
	}
	now := s.now()
	repositoryID, insightType, err := parseInsightKey(insightKey)
	if err != nil {
		return StatusResult{}, err
	}
	return s.repository.UpsertStatus(ctx, upsertStatusParams{
		OrganizationID: organizationID,
		RepositoryID:   optionalString(repositoryID),
		InsightKey:     insightKey,
		InsightType:    insightType,
		Status:         StatusOpen,
		ReopenedAt:     &now,
		UpdatedAt:      now,
	})
}

func (s *Service) generateRepositoryInsights(ctx context.Context, repo repositoryRecord, from time.Time, to time.Time) ([]Insight, error) {
	builders := []func(context.Context, string, time.Time, time.Time) ([]Insight, error){
		s.repository.ListBottlenecks,
		s.repository.ListLargePullRequests,
		s.repository.ListSlowReviews,
		s.repository.ListHotspots,
		s.repository.ListDeploymentFailureTrends,
		s.repository.ListReviewConcentration,
	}

	items := make([]Insight, 0)
	for _, build := range builders {
		result, err := build(ctx, repo.ID, from, to)
		if err != nil {
			return nil, err
		}
		items = append(items, result...)
	}
	return items, nil
}

func validateListParams(params ListParams) error {
	var issues []ValidationIssue
	if strings.TrimSpace(params.OrganizationID) == "" {
		issues = append(issues, ValidationIssue{Field: "organizationId", Message: "is required"})
	}
	if params.RepositoryID != "" && !isUUID(params.RepositoryID) {
		issues = append(issues, ValidationIssue{Field: "repositoryId", Message: "must be a valid UUID"})
	}
	if params.Page < 1 {
		issues = append(issues, ValidationIssue{Field: "page", Message: "must be greater than or equal to 1"})
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		issues = append(issues, ValidationIssue{Field: "pageSize", Message: "must be between 1 and 100"})
	}
	if params.From.IsZero() {
		issues = append(issues, ValidationIssue{Field: "from", Message: "is required"})
	}
	if params.To.IsZero() {
		issues = append(issues, ValidationIssue{Field: "to", Message: "is required"})
	}
	if !params.From.IsZero() && !params.To.IsZero() && params.To.Before(params.From) {
		issues = append(issues, ValidationIssue{Field: "to", Message: "must be greater than or equal to from"})
	}
	if params.Type != "" && !isValidType(params.Type) {
		issues = append(issues, ValidationIssue{Field: "type", Message: "must be a supported insight type"})
	}
	if params.Status != "" && params.Status != StatusOpen && params.Status != StatusReviewed && params.Status != StatusDismissed {
		issues = append(issues, ValidationIssue{Field: "status", Message: "must be one of open, reviewed, dismissed"})
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func filterInsights(items []Insight, insightType string, status string) []Insight {
	filtered := make([]Insight, 0, len(items))
	for _, item := range items {
		if insightType != "" && item.InsightType != insightType {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func applyStatus(item *Insight, status statusRecord) {
	item.Status = status.Status
	item.ReviewedBy = status.ReviewedBy
	item.ReviewedAt = status.ReviewedAt
	item.DismissedAt = status.DismissedAt
	item.ReopenedAt = status.ReopenedAt
}

func shouldReopen(status statusRecord, detectedAt time.Time) bool {
	if status.Status == StatusOpen {
		return false
	}
	cutoff := latestTime(status.ReviewedAt, status.DismissedAt, status.ReopenedAt)
	if cutoff == nil {
		return true
	}
	return detectedAt.After(*cutoff)
}

func latestTime(values ...*time.Time) *time.Time {
	var latest *time.Time
	for _, value := range values {
		if value == nil {
			continue
		}
		if latest == nil || value.After(*latest) {
			candidate := value.UTC()
			latest = &candidate
		}
	}
	return latest
}

func buildInsightKey(insightType string, repositoryID string, entity string) string {
	return fmt.Sprintf("%s:%s:%s", insightType, repositoryID, entity)
}

func parseInsightKey(insightKey string) (string, string, error) {
	parts := strings.SplitN(insightKey, ":", 3)
	if len(parts) != 3 {
		return "", "", &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "insightKey", Message: "must be a valid insight key"}},
		}
	}
	if !isValidType(parts[0]) {
		return "", "", &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "insightKey", Message: "contains unsupported insight type"}},
		}
	}
	if !isUUID(parts[1]) {
		return "", "", &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "insightKey", Message: "contains invalid repository id"}},
		}
	}
	return parts[1], parts[0], nil
}

func isUUID(value string) bool {
	if value == "" {
		return false
	}
	parts := strings.Split(value, "-")
	return len(parts) == 5
}

func isValidType(value string) bool {
	switch value {
	case TypeBottleneckDetection,
		TypeLargePRDetection,
		TypeSlowReviewDetection,
		TypeHotspotDetection,
		TypeDeploymentFailureTrend,
		TypeReviewConcentration:
		return true
	default:
		return false
	}
}

func severityRank(value string) int {
	switch value {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}

func endExclusive(value time.Time) time.Time {
	return value.UTC().Add(24 * time.Hour)
}

func insightKeys(items []Insight) []string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.InsightKey)
	}
	return keys
}

func hashKeyPart(value string) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(sum[:8])
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func timePtr(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}
