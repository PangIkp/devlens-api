package insights

import (
	"context"
	"testing"
	"time"
)

type stubStore struct {
	ensureOrganizationExistsFn func(context.Context, string) error
	ensureRepositoryFn         func(context.Context, string, string) error
	listRepositoriesFn         func(context.Context, string, string) ([]repositoryRecord, error)
	listBottlenecksFn          func(context.Context, string, time.Time, time.Time) ([]Insight, error)
	listLargePRsFn             func(context.Context, string, time.Time, time.Time) ([]Insight, error)
	listSlowReviewsFn          func(context.Context, string, time.Time, time.Time) ([]Insight, error)
	listHotspotsFn             func(context.Context, string, time.Time, time.Time) ([]Insight, error)
	listDeploymentFn           func(context.Context, string, time.Time, time.Time) ([]Insight, error)
	listReviewConcentrationFn  func(context.Context, string, time.Time, time.Time) ([]Insight, error)
	listStatusesFn             func(context.Context, string, []string) (map[string]statusRecord, error)
	getStatusByKeyFn           func(context.Context, string, string) (statusRecord, error)
	upsertStatusFn             func(context.Context, upsertStatusParams) (StatusResult, error)
}

func (s stubStore) EnsureOrganizationExists(ctx context.Context, id string) error {
	if s.ensureOrganizationExistsFn != nil {
		return s.ensureOrganizationExistsFn(ctx, id)
	}
	return nil
}
func (s stubStore) EnsureRepositoryInOrganization(ctx context.Context, orgID string, repoID string) error {
	if s.ensureRepositoryFn != nil {
		return s.ensureRepositoryFn(ctx, orgID, repoID)
	}
	return nil
}
func (s stubStore) ListRepositories(ctx context.Context, orgID string, repoID string) ([]repositoryRecord, error) {
	return s.listRepositoriesFn(ctx, orgID, repoID)
}
func (s stubStore) ListLargePullRequests(ctx context.Context, repoID string, from, to time.Time) ([]Insight, error) {
	if s.listLargePRsFn != nil {
		return s.listLargePRsFn(ctx, repoID, from, to)
	}
	return nil, nil
}
func (s stubStore) ListSlowReviews(ctx context.Context, repoID string, from, to time.Time) ([]Insight, error) {
	if s.listSlowReviewsFn != nil {
		return s.listSlowReviewsFn(ctx, repoID, from, to)
	}
	return nil, nil
}
func (s stubStore) ListHotspots(ctx context.Context, repoID string, from, to time.Time) ([]Insight, error) {
	if s.listHotspotsFn != nil {
		return s.listHotspotsFn(ctx, repoID, from, to)
	}
	return nil, nil
}
func (s stubStore) ListDeploymentFailureTrends(ctx context.Context, repoID string, from, to time.Time) ([]Insight, error) {
	if s.listDeploymentFn != nil {
		return s.listDeploymentFn(ctx, repoID, from, to)
	}
	return nil, nil
}
func (s stubStore) ListReviewConcentration(ctx context.Context, repoID string, from, to time.Time) ([]Insight, error) {
	if s.listReviewConcentrationFn != nil {
		return s.listReviewConcentrationFn(ctx, repoID, from, to)
	}
	return nil, nil
}
func (s stubStore) ListBottlenecks(ctx context.Context, repoID string, from, to time.Time) ([]Insight, error) {
	if s.listBottlenecksFn != nil {
		return s.listBottlenecksFn(ctx, repoID, from, to)
	}
	return nil, nil
}
func (s stubStore) ListStatusesByKeys(ctx context.Context, orgID string, keys []string) (map[string]statusRecord, error) {
	if s.listStatusesFn != nil {
		return s.listStatusesFn(ctx, orgID, keys)
	}
	return map[string]statusRecord{}, nil
}
func (s stubStore) GetStatusByKey(ctx context.Context, orgID string, key string) (statusRecord, error) {
	if s.getStatusByKeyFn != nil {
		return s.getStatusByKeyFn(ctx, orgID, key)
	}
	return statusRecord{}, nil
}
func (s stubStore) UpsertStatus(ctx context.Context, params upsertStatusParams) (StatusResult, error) {
	if s.upsertStatusFn != nil {
		return s.upsertStatusFn(ctx, params)
	}
	return StatusResult{}, nil
}

func TestListAppliesStoredStatus(t *testing.T) {
	t.Parallel()

	detectedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	reviewedAt := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	service := NewService(stubStore{
		listRepositoriesFn: func(context.Context, string, string) ([]repositoryRecord, error) {
			return []repositoryRecord{{ID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", FullName: "acme/api"}}, nil
		},
		listLargePRsFn: func(context.Context, string, time.Time, time.Time) ([]Insight, error) {
			return []Insight{{
				InsightKey:   buildInsightKey(TypeLargePRDetection, "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", "pr-12"),
				InsightType:  TypeLargePRDetection,
				Severity:     SeverityHigh,
				DetectedAt:   detectedAt,
				RepositoryID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
			}}, nil
		},
		listStatusesFn: func(context.Context, string, []string) (map[string]statusRecord, error) {
			return map[string]statusRecord{
				buildInsightKey(TypeLargePRDetection, "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", "pr-12"): {
					Status:     StatusReviewed,
					ReviewedAt: &reviewedAt,
				},
			}, nil
		},
	})

	result, err := service.List(context.Background(), ListParams{
		OrganizationID: "bd546e60-e65d-b1fd-3713-6f56aa60f149",
		From:           time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		To:             time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		Page:           1,
		PageSize:       20,
	})
	if err != nil {
		t.Fatalf("list insights: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(result.Items))
	}
	if result.Items[0].Status != StatusReviewed {
		t.Fatalf("expected reviewed status, got %s", result.Items[0].Status)
	}
}

func TestReviewParsesInsightKey(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		upsertStatusFn: func(_ context.Context, params upsertStatusParams) (StatusResult, error) {
			if params.InsightType != TypeLargePRDetection {
				t.Fatalf("unexpected insight type %s", params.InsightType)
			}
			if params.RepositoryID == nil || *params.RepositoryID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected repository id %+v", params.RepositoryID)
			}
			return StatusResult{InsightKey: params.InsightKey, InsightType: params.InsightType, Status: params.Status, UpdatedAt: params.UpdatedAt}, nil
		},
	})

	_, err := service.Review(context.Background(), "bd546e60-e65d-b1fd-3713-6f56aa60f149", buildInsightKey(TypeLargePRDetection, "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", "pr-12"), ReviewRequest{})
	if err != nil {
		t.Fatalf("review insight: %v", err)
	}
}
