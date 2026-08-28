package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/go-chi/chi/v5"
)

func newStubSyncJobService() stubSyncJobService {
	return stubSyncJobService{
		createFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		enqueueFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		getFn: func(context.Context, string) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		listFn: func(context.Context, syncjob.ListParams) (syncjob.ListResult, error) {
			return syncjob.ListResult{}, nil
		},
		retryFn: func(context.Context, string) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		cancelFn: func(context.Context, string) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
	}
}

type stubSyncJobService struct {
	createFn  func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error)
	enqueueFn func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error)
	getFn     func(context.Context, string) (syncjob.SyncJobResponse, error)
	listFn    func(context.Context, syncjob.ListParams) (syncjob.ListResult, error)
	retryFn   func(context.Context, string) (syncjob.SyncJobResponse, error)
	cancelFn  func(context.Context, string) (syncjob.SyncJobResponse, error)
}

func (s stubSyncJobService) Create(ctx context.Context, repositoryID string, req syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
	return s.createFn(ctx, repositoryID, req)
}

func (s stubSyncJobService) Enqueue(ctx context.Context, repositoryID string, req syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
	return s.enqueueFn(ctx, repositoryID, req)
}

func (s stubSyncJobService) GetByID(ctx context.Context, id string) (syncjob.SyncJobResponse, error) {
	return s.getFn(ctx, id)
}

func (s stubSyncJobService) ListByRepository(ctx context.Context, params syncjob.ListParams) (syncjob.ListResult, error) {
	return s.listFn(ctx, params)
}

func (s stubSyncJobService) Retry(ctx context.Context, id string) (syncjob.SyncJobResponse, error) {
	return s.retryFn(ctx, id)
}

func (s stubSyncJobService) Cancel(ctx context.Context, id string) (syncjob.SyncJobResponse, error) {
	return s.cancelFn(ctx, id)
}

func TestCreateSyncJobHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewSyncJobHandler(stubSyncJobService{
		createFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			t.Fatal("create should not be called")
			return syncjob.SyncJobResponse{}, nil
		},
		enqueueFn: func(_ context.Context, repositoryID string, req syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			if repositoryID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" || req.Mode != "full" {
				t.Fatalf("unexpected request %q %+v", repositoryID, req)
			}
			if req.IdempotencyKey != "idem-1" {
				t.Fatalf("expected idempotency key, got %q", req.IdempotencyKey)
			}
			return syncjob.SyncJobResponse{
				ID:           "5fb3c674-6992-4ba9-a227-c1c66517e3f6",
				RepositoryID: repositoryID,
				Status:       syncjob.StatusCompleted,
				Progress:     100,
				CreatedAt:    time.Now().UTC(),
			}, nil
		},
		getFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		listFn: func(context.Context, syncjob.ListParams) (syncjob.ListResult, error) {
			return syncjob.ListResult{}, nil
		},
		retryFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		cancelFn: func(context.Context, string) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/sync", strings.NewReader(`{"mode":"full"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestListSyncJobsHandlerUsesMetaShape(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewSyncJobHandler(stubSyncJobService{
		createFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		enqueueFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		getFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		listFn: func(_ context.Context, params syncjob.ListParams) (syncjob.ListResult, error) {
			if params.Page != 2 || params.PageSize != 10 || params.Status != "completed" || params.SortOrder != "asc" {
				t.Fatalf("unexpected params %+v", params)
			}
			return syncjob.ListResult{
				Items: []syncjob.SyncJobResponse{{
					ID:           "job-1",
					RepositoryID: params.RepositoryID,
					Status:       syncjob.StatusCompleted,
					Progress:     100,
					CreatedAt:    time.Now().UTC(),
				}},
				TotalItems: 1,
			}, nil
		},
		retryFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		cancelFn: func(context.Context, string) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/sync-jobs?page=2&pageSize=10&status=completed&sortOrder=asc", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Data []syncjob.SyncJobResponse `json:"data"`
		Meta PaginationMeta            `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Data) != 1 || body.Meta.Page != 2 {
		t.Fatalf("unexpected body %+v", body)
	}
}

func TestGetSyncJobHandlerNotFound(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewSyncJobHandler(stubSyncJobService{
		createFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		enqueueFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		getFn: func(context.Context, string) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, syncjob.ErrSyncJobNotFound
		},
		listFn: func(context.Context, syncjob.ListParams) (syncjob.ListResult, error) {
			return syncjob.ListResult{}, nil
		},
		retryFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		cancelFn: func(context.Context, string) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/sync-jobs/5fb3c674-6992-4ba9-a227-c1c66517e3f6", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRetrySyncJobHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewSyncJobHandler(stubSyncJobService{
		createFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		enqueueFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		getFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		listFn: func(context.Context, syncjob.ListParams) (syncjob.ListResult, error) {
			return syncjob.ListResult{}, nil
		},
		retryFn: func(_ context.Context, id string) (syncjob.SyncJobResponse, error) {
			if id != "5fb3c674-6992-4ba9-a227-c1c66517e3f6" {
				t.Fatalf("unexpected sync job id %q", id)
			}
			return syncjob.SyncJobResponse{
				ID:           id,
				RepositoryID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
				Status:       syncjob.StatusPending,
				Progress:     0,
				CreatedAt:    time.Now().UTC(),
			}, nil
		},
		cancelFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/sync-jobs/5fb3c674-6992-4ba9-a227-c1c66517e3f6/retry", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestCancelSyncJobHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewSyncJobHandler(stubSyncJobService{
		createFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		enqueueFn: func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			return syncjob.SyncJobResponse{}, nil
		},
		getFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		listFn: func(context.Context, syncjob.ListParams) (syncjob.ListResult, error) {
			return syncjob.ListResult{}, nil
		},
		retryFn: func(context.Context, string) (syncjob.SyncJobResponse, error) { return syncjob.SyncJobResponse{}, nil },
		cancelFn: func(_ context.Context, id string) (syncjob.SyncJobResponse, error) {
			if id != "5fb3c674-6992-4ba9-a227-c1c66517e3f6" {
				t.Fatalf("unexpected sync job id %q", id)
			}
			return syncjob.SyncJobResponse{
				ID:           id,
				RepositoryID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
				Status:       syncjob.StatusCanceled,
				Progress:     25,
				CreatedAt:    time.Now().UTC(),
			}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/sync-jobs/5fb3c674-6992-4ba9-a227-c1c66517e3f6/cancel", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestCreateSyncJobHandlerRejectsRepositoryWithoutOnboarding(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	service := newStubSyncJobService()
	service.enqueueFn = func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
		return syncjob.SyncJobResponse{}, syncjob.ErrRepositoryNotSelected
	}
	NewSyncJobHandler(service).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/sync", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != errorCodeRepositoryOnboardingRequired {
		t.Fatalf("expected code %q, got %q", errorCodeRepositoryOnboardingRequired, body.Error.Code)
	}
	if !strings.Contains(body.Error.Message, "Repository must be selected") {
		t.Fatalf("unexpected body %+v", body)
	}
}

func TestCreateSyncJobHandlerRejectsRepositoryWithoutConnectedInstallation(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	service := newStubSyncJobService()
	service.enqueueFn = func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
		return syncjob.SyncJobResponse{}, syncjob.ErrRepositoryNotConnected
	}
	NewSyncJobHandler(service).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/sync", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != errorCodeGitHubInstallationRequired {
		t.Fatalf("expected code %q, got %q", errorCodeGitHubInstallationRequired, body.Error.Code)
	}
	if !strings.Contains(body.Error.Message, "GitHub installation must be connected") {
		t.Fatalf("unexpected body %+v", body)
	}
}

func TestCreateSyncJobHandlerMapsInvalidGitHubAppCredentials(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	service := newStubSyncJobService()
	service.enqueueFn = func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
		return syncjob.SyncJobResponse{}, syncjob.ErrGitHubAppCredentialsInvalid
	}
	NewSyncJobHandler(service).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/repositories/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/sync", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error.Code != errorCodeGitHubAppCredentialsInvalid {
		t.Fatalf("expected code %q, got %q", errorCodeGitHubAppCredentialsInvalid, body.Error.Code)
	}
	if strings.Contains(body.Error.Message, "github app request failed") {
		t.Fatalf("expected sanitized body %+v", body)
	}
}
