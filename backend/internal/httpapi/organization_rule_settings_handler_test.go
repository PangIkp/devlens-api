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

	"github.com/PangIkp/devlens/backend/internal/orgrulesettings"
)

type stubOrganizationRuleSettingsService struct {
	getFn    func(context.Context, string) (orgrulesettings.Response, error)
	updateFn func(context.Context, string, orgrulesettings.UpdateRequest, *string) (orgrulesettings.Response, error)
}

func (s stubOrganizationRuleSettingsService) Get(ctx context.Context, organizationID string) (orgrulesettings.Response, error) {
	return s.getFn(ctx, organizationID)
}

func (s stubOrganizationRuleSettingsService) Update(ctx context.Context, organizationID string, req orgrulesettings.UpdateRequest, actorID *string) (orgrulesettings.Response, error) {
	return s.updateFn(ctx, organizationID, req, actorID)
}

func TestGetOrganizationRuleSettingsHandler(t *testing.T) {
	t.Parallel()

	service := stubOrganizationRuleSettingsService{
		getFn: func(context.Context, string) (orgrulesettings.Response, error) {
			return orgrulesettings.Response{
				LargePR: orgrulesettings.LargePRSettings{Enabled: true, FilesThreshold: 25, TotalChangesThreshold: 800},
			}, nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:                 stubHealthChecker{},
		OrganizationRuleSettings: NewOrganizationRuleSettingsHandler(service),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/settings/rules", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body DataResponse[orgrulesettings.Response]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.LargePR.FilesThreshold != 25 {
		t.Fatalf("expected filesThreshold 25, got %d", body.Data.LargePR.FilesThreshold)
	}
}

func TestUpdateOrganizationRuleSettingsHandler(t *testing.T) {
	t.Parallel()

	var gotOrgID string
	var gotReq orgrulesettings.UpdateRequest
	service := stubOrganizationRuleSettingsService{
		updateFn: func(_ context.Context, organizationID string, req orgrulesettings.UpdateRequest, _ *string) (orgrulesettings.Response, error) {
			gotOrgID = organizationID
			gotReq = req
			return orgrulesettings.Response{
				LargePR: orgrulesettings.LargePRSettings{Enabled: true, FilesThreshold: 40, TotalChangesThreshold: 800},
			}, nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:                 stubHealthChecker{},
		OrganizationRuleSettings: NewOrganizationRuleSettingsHandler(service),
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/settings/rules", strings.NewReader(`{"largePR":{"filesThreshold":40}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if gotOrgID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
		t.Fatalf("unexpected organization id %q", gotOrgID)
	}
	if gotReq.LargePR == nil || gotReq.LargePR.FilesThreshold == nil || *gotReq.LargePR.FilesThreshold != 40 {
		t.Fatalf("expected decoded largePR.filesThreshold 40, got %+v", gotReq.LargePR)
	}

	var body DataResponse[orgrulesettings.Response]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.LargePR.FilesThreshold != 40 {
		t.Fatalf("expected filesThreshold 40, got %d", body.Data.LargePR.FilesThreshold)
	}
}

func TestGetOrganizationRuleSettingsHandlerReturnsNotFound(t *testing.T) {
	t.Parallel()

	service := stubOrganizationRuleSettingsService{
		getFn: func(context.Context, string) (orgrulesettings.Response, error) {
			return orgrulesettings.Response{}, orgrulesettings.ErrOrganizationNotFound
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:                 stubHealthChecker{},
		OrganizationRuleSettings: NewOrganizationRuleSettingsHandler(service),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/settings/rules", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}
