package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/insights"
	"github.com/go-chi/chi/v5"
)

type stubInsightService struct {
	listFn    func(context.Context, insights.ListParams) (insights.ListResult, error)
	reviewFn  func(context.Context, string, string, insights.ReviewRequest) (insights.StatusResult, error)
	dismissFn func(context.Context, string, string) (insights.StatusResult, error)
	reopenFn  func(context.Context, string, string) (insights.StatusResult, error)
}

func (s stubInsightService) List(ctx context.Context, params insights.ListParams) (insights.ListResult, error) {
	return s.listFn(ctx, params)
}
func (s stubInsightService) Review(ctx context.Context, organizationID string, insightKey string, req insights.ReviewRequest) (insights.StatusResult, error) {
	return s.reviewFn(ctx, organizationID, insightKey, req)
}
func (s stubInsightService) Dismiss(ctx context.Context, organizationID string, insightKey string) (insights.StatusResult, error) {
	return s.dismissFn(ctx, organizationID, insightKey)
}
func (s stubInsightService) Reopen(ctx context.Context, organizationID string, insightKey string) (insights.StatusResult, error) {
	return s.reopenFn(ctx, organizationID, insightKey)
}

func TestInsightHandlerList(t *testing.T) {
	t.Parallel()

	handler := NewInsightHandler(stubInsightService{
		listFn: func(_ context.Context, params insights.ListParams) (insights.ListResult, error) {
			if params.Status != insights.StatusOpen {
				t.Fatalf("unexpected status filter %s", params.Status)
			}
			return insights.ListResult{
				Items: []insights.Insight{{
					InsightKey:  "large_pr_detection:8f1cd971-1fd9-4f4f-9f75-47f6ed14938d:pr-12",
					InsightType: insights.TypeLargePRDetection,
					Status:      insights.StatusOpen,
					Severity:    insights.SeverityHigh,
					DetectedAt:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				}},
				TotalItems: 1,
			}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations/bd546e60-e65d-b1fd-3713-6f56aa60f149/insights?from=2026-08-01&to=2026-08-07&page=1&pageSize=10&status=open", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["pagination"]; !ok {
		t.Fatalf("expected pagination in response: %s", rec.Body.String())
	}
}

func TestInsightHandlerListAlias(t *testing.T) {
	t.Parallel()

	handler := NewInsightHandler(stubInsightService{
		listFn: func(_ context.Context, params insights.ListParams) (insights.ListResult, error) {
			if params.OrganizationID != "bd546e60-e65d-b1fd-3713-6f56aa60f149" {
				t.Fatalf("unexpected organization id %s", params.OrganizationID)
			}
			return insights.ListResult{}, nil
		},
		reviewFn: func(context.Context, string, string, insights.ReviewRequest) (insights.StatusResult, error) {
			return insights.StatusResult{}, nil
		},
		dismissFn: func(context.Context, string, string) (insights.StatusResult, error) {
			return insights.StatusResult{}, nil
		},
		reopenFn: func(context.Context, string, string) (insights.StatusResult, error) {
			return insights.StatusResult{}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/insights?organizationId=bd546e60-e65d-b1fd-3713-6f56aa60f149&from=2026-08-01&to=2026-08-07&page=1&pageSize=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInsightHandlerReview(t *testing.T) {
	t.Parallel()

	handler := NewInsightHandler(stubInsightService{
		listFn: func(context.Context, insights.ListParams) (insights.ListResult, error) {
			return insights.ListResult{}, nil
		},
		reviewFn: func(_ context.Context, organizationID string, insightKey string, req insights.ReviewRequest) (insights.StatusResult, error) {
			if organizationID != "bd546e60-e65d-b1fd-3713-6f56aa60f149" {
				t.Fatalf("unexpected organization id %s", organizationID)
			}
			if insightKey == "" {
				t.Fatal("expected insight key")
			}
			if req.ReviewedBy == nil || *req.ReviewedBy != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected reviewedBy %+v", req.ReviewedBy)
			}
			return insights.StatusResult{InsightKey: insightKey, InsightType: insights.TypeLargePRDetection, Status: insights.StatusReviewed, UpdatedAt: time.Now().UTC()}, nil
		},
		dismissFn: func(context.Context, string, string) (insights.StatusResult, error) {
			return insights.StatusResult{}, nil
		},
		reopenFn: func(context.Context, string, string) (insights.StatusResult, error) {
			return insights.StatusResult{}, nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	payload := []byte(`{"reviewedBy":"8f1cd971-1fd9-4f4f-9f75-47f6ed14938d"}`)
	req := httptest.NewRequest(http.MethodPost, "/organizations/bd546e60-e65d-b1fd-3713-6f56aa60f149/insights/large_pr_detection:8f1cd971-1fd9-4f4f-9f75-47f6ed14938d:pr-12/review", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
