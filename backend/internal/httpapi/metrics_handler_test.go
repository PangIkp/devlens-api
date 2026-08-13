package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/go-chi/chi/v5"
)

type stubMetricsService struct {
	getHotspotsFn             func(context.Context, string, metrics.HotspotQueryParams) (metrics.HotspotResult, error)
	getRepositoryMetricsFn    func(context.Context, string, metrics.DeploymentQueryParams) (metrics.RepositoryMetrics, error)
	getReviewQueueFn          func(context.Context, string, metrics.HotspotQueryParams) (metrics.ReviewQueueResult, error)
	getWorkloadDistributionFn func(context.Context, string, metrics.QueryParams) (metrics.WorkloadDistribution, error)
}

func (s stubMetricsService) GetDashboardSummary(context.Context, string, metrics.QueryParams) (metrics.DashboardSummary, error) {
	return metrics.DashboardSummary{}, nil
}

func (s stubMetricsService) GetPullRequestMetrics(context.Context, string, metrics.QueryParams) (metrics.PullRequestMetrics, error) {
	return metrics.PullRequestMetrics{}, nil
}

func (s stubMetricsService) GetReviewMetrics(context.Context, string, metrics.QueryParams) (metrics.ReviewMetrics, error) {
	return metrics.ReviewMetrics{}, nil
}

func (s stubMetricsService) GetDeploymentMetrics(context.Context, string, metrics.DeploymentQueryParams) (metrics.DeploymentMetrics, error) {
	return metrics.DeploymentMetrics{}, nil
}

func (s stubMetricsService) GetWorkloadDistribution(ctx context.Context, repositoryID string, params metrics.QueryParams) (metrics.WorkloadDistribution, error) {
	return s.getWorkloadDistributionFn(ctx, repositoryID, params)
}

func (s stubMetricsService) GetHotspots(ctx context.Context, repositoryID string, params metrics.HotspotQueryParams) (metrics.HotspotResult, error) {
	return s.getHotspotsFn(ctx, repositoryID, params)
}

func (s stubMetricsService) GetRepositoryMetrics(ctx context.Context, repositoryID string, params metrics.DeploymentQueryParams) (metrics.RepositoryMetrics, error) {
	return s.getRepositoryMetricsFn(ctx, repositoryID, params)
}

func (s stubMetricsService) GetReviewQueue(ctx context.Context, repositoryID string, params metrics.HotspotQueryParams) (metrics.ReviewQueueResult, error) {
	return s.getReviewQueueFn(ctx, repositoryID, params)
}

func TestMetricsHandlerHotspotsUsesMetaField(t *testing.T) {
	t.Parallel()

	handler := NewMetricsHandler(stubMetricsService{
		getHotspotsFn: func(_ context.Context, repositoryID string, params metrics.HotspotQueryParams) (metrics.HotspotResult, error) {
			if repositoryID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected repository id %s", repositoryID)
			}
			if params.SortOrder != "desc" {
				t.Fatalf("unexpected sort order %s", params.SortOrder)
			}
			return metrics.HotspotResult{
				Items:      []metrics.HotspotFile{{FilePath: "internal/metrics/service.go", HotspotScore: 12}},
				TotalItems: 1,
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/metrics/hotspots?from=2026-07-01&to=2026-07-07&page=1&pageSize=10", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["meta"]; !ok {
		t.Fatalf("expected meta field in response, got %s", rec.Body.String())
	}
	if _, ok := body["pagination"]; ok {
		t.Fatalf("did not expect pagination field in response")
	}
}

func TestMetricsHandlerRequiresDateRange(t *testing.T) {
	t.Parallel()

	handler := NewMetricsHandler(stubMetricsService{
		getHotspotsFn: func(context.Context, string, metrics.HotspotQueryParams) (metrics.HotspotResult, error) {
			t.Fatal("service should not be called")
			return metrics.HotspotResult{}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/metrics/hotspots?page=1&pageSize=10", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestMetricsHandlerRepositoryMetrics(t *testing.T) {
	t.Parallel()

	handler := NewMetricsHandler(stubMetricsService{
		getHotspotsFn: func(context.Context, string, metrics.HotspotQueryParams) (metrics.HotspotResult, error) {
			return metrics.HotspotResult{}, nil
		},
		getRepositoryMetricsFn: func(_ context.Context, repositoryID string, params metrics.DeploymentQueryParams) (metrics.RepositoryMetrics, error) {
			if repositoryID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected repository id %s", repositoryID)
			}
			if params.DayType != metrics.DayTypeBusiness {
				t.Fatalf("expected dayType=%q, got %q", metrics.DayTypeBusiness, params.DayType)
			}
			return metrics.RepositoryMetrics{
				MetricVersion: metrics.CurrentMetricVersion,
				DayType:       params.DayType,
				RepositoryID:  repositoryID,
				From:          "2026-07-01",
				To:            "2026-07-07",
				Interval:      "day",
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/metrics?from=2026-07-01&to=2026-07-07&interval=day&dayType=business", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsHandlerRejectsInvalidDayType(t *testing.T) {
	t.Parallel()

	handler := NewMetricsHandler(stubMetricsService{
		getRepositoryMetricsFn: func(_ context.Context, _ string, params metrics.DeploymentQueryParams) (metrics.RepositoryMetrics, error) {
			if params.DayType != "weird" {
				t.Fatalf("expected raw dayType to reach service, got %q", params.DayType)
			}
			return metrics.RepositoryMetrics{}, &metrics.ValidationError{
				Message: "request validation failed",
				Details: []metrics.ValidationIssue{{Field: "dayType", Message: "must be one of calendar, business"}},
			}
		},
		getHotspotsFn: func(context.Context, string, metrics.HotspotQueryParams) (metrics.HotspotResult, error) {
			return metrics.HotspotResult{}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/metrics?from=2026-07-01&to=2026-07-07&interval=day&dayType=weird", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsHandlerReviewQueueUsesMetaField(t *testing.T) {
	t.Parallel()

	handler := NewMetricsHandler(stubMetricsService{
		getHotspotsFn: func(context.Context, string, metrics.HotspotQueryParams) (metrics.HotspotResult, error) {
			return metrics.HotspotResult{}, nil
		},
		getRepositoryMetricsFn: func(context.Context, string, metrics.DeploymentQueryParams) (metrics.RepositoryMetrics, error) {
			return metrics.RepositoryMetrics{}, nil
		},
		getReviewQueueFn: func(_ context.Context, repositoryID string, params metrics.HotspotQueryParams) (metrics.ReviewQueueResult, error) {
			if repositoryID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected repository id %s", repositoryID)
			}
			return metrics.ReviewQueueResult{
				Items:      []metrics.ReviewQueueItem{{PullRequestID: "pr-1", Number: 12, Title: "Queue", Author: "itsara"}},
				TotalItems: 1,
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/dashboard/review-queue?from=2026-07-01&to=2026-07-07&page=1&pageSize=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsHandlerWorkloadDistribution(t *testing.T) {
	t.Parallel()

	handler := NewMetricsHandler(stubMetricsService{
		getHotspotsFn: func(context.Context, string, metrics.HotspotQueryParams) (metrics.HotspotResult, error) {
			return metrics.HotspotResult{}, nil
		},
		getRepositoryMetricsFn: func(context.Context, string, metrics.DeploymentQueryParams) (metrics.RepositoryMetrics, error) {
			return metrics.RepositoryMetrics{}, nil
		},
		getReviewQueueFn: func(context.Context, string, metrics.HotspotQueryParams) (metrics.ReviewQueueResult, error) {
			return metrics.ReviewQueueResult{}, nil
		},
		getWorkloadDistributionFn: func(_ context.Context, repositoryID string, params metrics.QueryParams) (metrics.WorkloadDistribution, error) {
			if repositoryID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected repository id %s", repositoryID)
			}
			if params.From.Format("2006-01-02") != "2026-07-01" || params.To.Format("2006-01-02") != "2026-07-07" {
				t.Fatalf("unexpected date range %s - %s", params.From, params.To)
			}
			return metrics.WorkloadDistribution{
				Summary: metrics.WorkloadDistributionSummary{
					RepositoryID:        repositoryID,
					From:                "2026-07-01",
					To:                  "2026-07-07",
					TotalPullRequests:   4,
					TotalReviews:        5,
					TopContributorShare: 0.5,
					TopReviewerShare:    0.6,
				},
				Contributors: []metrics.ContributorDistributionItem{{Author: "itsara", PullRequestCount: 2, Share: 0.5}},
				Reviewers:    []metrics.ReviewerDistributionItem{{Reviewer: "pangikp", ReviewCount: 3, ReviewedPullRequestCount: 2, Share: 0.6}},
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/metrics/workload-distribution?from=2026-07-01&to=2026-07-07", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data metrics.WorkloadDistribution `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Data.Contributors) != 1 || body.Data.Contributors[0].Author != "itsara" {
		t.Fatalf("unexpected contributors payload %+v", body.Data.Contributors)
	}
	if len(body.Data.Reviewers) != 1 || body.Data.Reviewers[0].Reviewer != "pangikp" {
		t.Fatalf("unexpected reviewers payload %+v", body.Data.Reviewers)
	}
}
