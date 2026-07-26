package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	devrepository "github.com/PangIkp/devlens/backend/internal/repository"
	"github.com/go-chi/chi/v5"
)

type stubRepositoryService struct {
	createFn func(context.Context, string, devrepository.CreateRepositoryRequest) (devrepository.RepositoryResponse, error)
	getFn    func(context.Context, string) (devrepository.RepositoryResponse, error)
	listFn   func(context.Context, devrepository.ListParams) (devrepository.ListResult, error)
	updateFn func(context.Context, string, devrepository.UpdateRepositoryRequest) (devrepository.RepositoryResponse, error)
}

func (s stubRepositoryService) Create(ctx context.Context, organizationID string, req devrepository.CreateRepositoryRequest) (devrepository.RepositoryResponse, error) {
	return s.createFn(ctx, organizationID, req)
}

func (s stubRepositoryService) GetByID(ctx context.Context, id string) (devrepository.RepositoryResponse, error) {
	return s.getFn(ctx, id)
}

func (s stubRepositoryService) List(ctx context.Context, params devrepository.ListParams) (devrepository.ListResult, error) {
	return s.listFn(ctx, params)
}

func (s stubRepositoryService) Update(ctx context.Context, id string, req devrepository.UpdateRepositoryRequest) (devrepository.RepositoryResponse, error) {
	return s.updateFn(ctx, id, req)
}

func TestCreateRepositoryHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewRepositoryHandler(stubRepositoryService{
		createFn: func(_ context.Context, organizationID string, req devrepository.CreateRepositoryRequest) (devrepository.RepositoryResponse, error) {
			if organizationID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected organization id %q", organizationID)
			}
			if req.GithubID != 42 || req.Name != "devlens-api" || req.FullName != "devlens-labs/devlens-api" {
				t.Fatalf("unexpected request %+v", req)
			}
			now := time.Now().UTC()
			return devrepository.RepositoryResponse{
				ID:             "5fb3c674-6992-4ba9-a227-c1c66517e3f6",
				OrganizationID: organizationID,
				GithubID:       req.GithubID,
				Name:           req.Name,
				FullName:       req.FullName,
				IsActive:       true,
				CreatedAt:      now,
			}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/repositories", strings.NewReader(`{"githubId":42,"name":"devlens-api","fullName":"devlens-labs/devlens-api"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestListRepositoriesHandlerPaginationAndFilters(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewRepositoryHandler(stubRepositoryService{
		listFn: func(_ context.Context, params devrepository.ListParams) (devrepository.ListResult, error) {
			if params.Page != 2 || params.PageSize != 10 || params.Status != "active" || params.Search != "devlens" || params.SortBy != "name" || params.SortOrder != "asc" {
				t.Fatalf("unexpected params %+v", params)
			}
			return devrepository.ListResult{TotalItems: 1}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/repositories?page=2&pageSize=10&status=active&search=devlens&sortBy=name&sortOrder=asc", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestListRepositoriesHandlerResponseShape(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewRepositoryHandler(stubRepositoryService{
		listFn: func(_ context.Context, _ devrepository.ListParams) (devrepository.ListResult, error) {
			return devrepository.ListResult{
				Items: []devrepository.RepositoryResponse{
					{
						ID:             "repo-1",
						OrganizationID: "org-1",
						GithubID:       42,
						Name:           "devlens-api",
						FullName:       "devlens-labs/devlens-api",
						IsActive:       true,
						CreatedAt:      time.Now().UTC(),
					},
				},
				TotalItems: 1,
			}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/repositories", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body struct {
		Data       []devrepository.RepositoryResponse `json:"data"`
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

func TestGetRepositoryHandlerNotFound(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewRepositoryHandler(stubRepositoryService{
		getFn: func(context.Context, string) (devrepository.RepositoryResponse, error) {
			return devrepository.RepositoryResponse{}, devrepository.ErrRepositoryNotFound
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/5fb3c674-6992-4ba9-a227-c1c66517e3f6", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateRepositoryHandlerValidationError(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewRepositoryHandler(stubRepositoryService{
		updateFn: func(context.Context, string, devrepository.UpdateRepositoryRequest) (devrepository.RepositoryResponse, error) {
			return devrepository.RepositoryResponse{}, &devrepository.ValidationError{
				Message: "request validation failed",
				Details: []devrepository.ValidationIssue{
					{Field: "body", Message: "must include at least one updatable field"},
				},
			}
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPatch, "/repositories/5fb3c674-6992-4ba9-a227-c1c66517e3f6", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
