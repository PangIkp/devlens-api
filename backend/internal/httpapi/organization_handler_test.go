package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/organization"
	"github.com/go-chi/chi/v5"
)

type stubOrganizationService struct {
	createFn func(context.Context, organization.CreateOrganizationRequest) (organization.OrganizationResponse, error)
	getFn    func(context.Context, string) (organization.OrganizationResponse, error)
	listFn   func(context.Context, organization.ListParams) (organization.ListResult, error)
}

func (s stubOrganizationService) Create(ctx context.Context, req organization.CreateOrganizationRequest) (organization.OrganizationResponse, error) {
	return s.createFn(ctx, req)
}

func (s stubOrganizationService) GetByID(ctx context.Context, id string) (organization.OrganizationResponse, error) {
	return s.getFn(ctx, id)
}

func (s stubOrganizationService) List(ctx context.Context, params organization.ListParams) (organization.ListResult, error) {
	return s.listFn(ctx, params)
}

func TestCreateOrganizationHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		createFn: func(_ context.Context, req organization.CreateOrganizationRequest) (organization.OrganizationResponse, error) {
			if req.GithubID != 123 || req.Slug != "devlens" || req.Name != "DevLens" {
				t.Fatalf("unexpected request %+v", req)
			}
			return organization.OrganizationResponse{
				ID:        "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
				GithubID:  123,
				Slug:      "devlens",
				Name:      "DevLens",
				CreatedAt: time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC),
			}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations", strings.NewReader(`{"githubId":123,"slug":"devlens","name":"DevLens"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestCreateOrganizationHandlerInvalidBody(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		createFn: func(_ context.Context, _ organization.CreateOrganizationRequest) (organization.OrganizationResponse, error) {
			t.Fatal("service should not be called")
			return organization.OrganizationResponse{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations", strings.NewReader(`{"githubId":"x"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetOrganizationHandlerInvalidUUID(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		getFn: func(_ context.Context, _ string) (organization.OrganizationResponse, error) {
			t.Fatal("service should not be called")
			return organization.OrganizationResponse{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations/not-a-uuid", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetOrganizationHandlerNotFound(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		getFn: func(_ context.Context, _ string) (organization.OrganizationResponse, error) {
			return organization.OrganizationResponse{}, organization.ErrOrganizationNotFound
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreateOrganizationHandlerDuplicate(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		createFn: func(_ context.Context, _ organization.CreateOrganizationRequest) (organization.OrganizationResponse, error) {
			return organization.OrganizationResponse{}, organization.ErrOrganizationConflict
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations", strings.NewReader(`{"githubId":123,"slug":"devlens","name":"DevLens"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestCreateOrganizationHandlerInvalidSlug(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		createFn: func(_ context.Context, _ organization.CreateOrganizationRequest) (organization.OrganizationResponse, error) {
			return organization.OrganizationResponse{}, &organization.ValidationError{
				Message: "request validation failed",
				Details: []organization.ValidationIssue{
					{
						Field:   "slug",
						Message: "must contain only lowercase letters, numbers, and hyphens",
					},
				},
			}
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations", strings.NewReader(`{"githubId":123,"slug":"DevLens","name":"DevLens"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListOrganizationsHandlerPaginationDefault(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		listFn: func(_ context.Context, params organization.ListParams) (organization.ListResult, error) {
			if params.Page != 1 || params.PageSize != 20 {
				t.Fatalf("unexpected params %+v", params)
			}
			return organization.ListResult{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestListOrganizationsHandlerPaginationInvalidValue(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		listFn: func(_ context.Context, _ organization.ListParams) (organization.ListResult, error) {
			t.Fatal("service should not be called")
			return organization.ListResult{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations?page=0", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListOrganizationsHandlerResponseShape(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		listFn: func(_ context.Context, _ organization.ListParams) (organization.ListResult, error) {
			return organization.ListResult{
				Items: []organization.OrganizationResponse{
					{ID: "1", GithubID: 1, Slug: "devlens", Name: "DevLens", CreatedAt: time.Now().UTC()},
				},
				TotalItems: 1,
			}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations?page=1&pageSize=20", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body struct {
		Data       []organization.OrganizationResponse `json:"data"`
		Pagination struct {
			Page int `json:"page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Data) != 1 || body.Pagination.Page != 1 {
		t.Fatalf("unexpected body %+v", body)
	}
}

func TestGetOrganizationHandlerInternalError(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationHandler(stubOrganizationService{
		getFn: func(_ context.Context, _ string) (organization.OrganizationResponse, error) {
			return organization.OrganizationResponse{}, errors.New("boom")
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
