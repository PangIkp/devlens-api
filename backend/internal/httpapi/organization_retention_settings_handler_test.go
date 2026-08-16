package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PangIkp/devlens/backend/internal/orgretention"
)

type stubOrganizationRetentionSettingsService struct {
	getFn    func(context.Context, string) (orgretention.Response, error)
	updateFn func(context.Context, string, orgretention.UpdateRequest, *string) (orgretention.Response, error)
}

func (s stubOrganizationRetentionSettingsService) Get(ctx context.Context, organizationID string) (orgretention.Response, error) {
	return s.getFn(ctx, organizationID)
}

func (s stubOrganizationRetentionSettingsService) Update(ctx context.Context, organizationID string, req orgretention.UpdateRequest, actorID *string) (orgretention.Response, error) {
	return s.updateFn(ctx, organizationID, req, actorID)
}

func TestGetOrganizationRetentionSettingsHandler(t *testing.T) {
	t.Parallel()

	service := stubOrganizationRetentionSettingsService{
		getFn: func(context.Context, string) (orgretention.Response, error) {
			return orgretention.Response{AnalyticsRawRetentionDays: 180, Enforced: false}, nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:                      stubHealthChecker{},
		OrganizationRetentionSettings: NewOrganizationRetentionSettingsHandler(service),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/settings/retention", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body DataResponse[orgretention.Response]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.AnalyticsRawRetentionDays != 180 {
		t.Fatalf("expected 180, got %d", body.Data.AnalyticsRawRetentionDays)
	}
	if body.Data.Enforced {
		t.Fatal("expected enforced to be false")
	}
}

func TestUpdateOrganizationRetentionSettingsHandler(t *testing.T) {
	t.Parallel()

	var gotDays *int
	service := stubOrganizationRetentionSettingsService{
		updateFn: func(_ context.Context, _ string, req orgretention.UpdateRequest, _ *string) (orgretention.Response, error) {
			gotDays = req.AnalyticsRawRetentionDays
			return orgretention.Response{AnalyticsRawRetentionDays: 45}, nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:                      stubHealthChecker{},
		OrganizationRetentionSettings: NewOrganizationRetentionSettingsHandler(service),
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/settings/retention", strings.NewReader(`{"analyticsRawRetentionDays":45}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if gotDays == nil || *gotDays != 45 {
		t.Fatalf("expected decoded days 45, got %v", gotDays)
	}
}

func TestUpdateOrganizationRetentionSettingsHandlerReturnsValidationError(t *testing.T) {
	t.Parallel()

	service := stubOrganizationRetentionSettingsService{
		updateFn: func(context.Context, string, orgretention.UpdateRequest, *string) (orgretention.Response, error) {
			return orgretention.Response{}, &orgretention.ValidationError{
				Message: "request validation failed",
				Details: []orgretention.ValidationIssue{{Field: "analyticsRawRetentionDays", Message: "must be greater than 0"}},
			}
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:                      stubHealthChecker{},
		OrganizationRetentionSettings: NewOrganizationRetentionSettingsHandler(service),
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/settings/retention", strings.NewReader(`{"analyticsRawRetentionDays":0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}
