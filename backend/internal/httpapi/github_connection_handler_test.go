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

	"github.com/PangIkp/devlens/backend/internal/githubconnection"
)

type stubGitHubConnectionService struct {
	getFn        func(context.Context, string) (githubconnection.ConnectionResponse, error)
	startFn      func(context.Context, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error)
	completeFn   func(context.Context, string, int64, string) (githubconnection.ConnectionResponse, error)
	listFn       func(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error)
	selectFn     func(context.Context, string, githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error)
	disconnectFn func(context.Context, string) error
}

func (s stubGitHubConnectionService) GetConnection(ctx context.Context, organizationID string) (githubconnection.ConnectionResponse, error) {
	return s.getFn(ctx, organizationID)
}

func (s stubGitHubConnectionService) StartInstallation(ctx context.Context, organizationID string, req githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error) {
	return s.startFn(ctx, organizationID, req)
}

func (s stubGitHubConnectionService) CompleteInstallation(ctx context.Context, organizationID string, installationID int64, state string) (githubconnection.ConnectionResponse, error) {
	return s.completeFn(ctx, organizationID, installationID, state)
}

func (s stubGitHubConnectionService) ListAccessibleRepositories(ctx context.Context, params githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error) {
	return s.listFn(ctx, params)
}

func (s stubGitHubConnectionService) SelectRepositories(ctx context.Context, organizationID string, req githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error) {
	return s.selectFn(ctx, organizationID, req)
}

func (s stubGitHubConnectionService) Disconnect(ctx context.Context, organizationID string) error {
	if s.disconnectFn == nil {
		return nil
	}
	return s.disconnectFn(ctx, organizationID)
}

func TestGetGitHubConnectionHandler(t *testing.T) {
	t.Parallel()

	service := stubGitHubConnectionService{
		getFn: func(_ context.Context, organizationID string) (githubconnection.ConnectionResponse, error) {
			if organizationID == "" {
				t.Fatal("expected organization id")
			}
			return githubconnection.ConnectionResponse{
				OrganizationID:        organizationID,
				Provider:              "github",
				State:                 githubconnection.StateConnected,
				ConnectedRepositories: 2,
			}, nil
		},
		startFn: func(context.Context, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error) {
			return githubconnection.StartInstallationResponse{}, nil
		},
		completeFn: func(context.Context, string, int64, string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{}, nil
		},
		listFn: func(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error) {
			return githubconnection.ListAccessibleRepositoriesResult{}, nil
		},
		selectFn: func(context.Context, string, githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error) {
			return githubconnection.SelectRepositoriesResponse{}, nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:          stubHealthChecker{},
		GitHubConnections: NewGitHubConnectionHandler(service),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/github/connection", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("expected ETag header")
	}
}

func TestSelectGitHubRepositoriesHandler(t *testing.T) {
	t.Parallel()

	service := stubGitHubConnectionService{
		getFn: func(context.Context, string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{}, nil
		},
		startFn: func(context.Context, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error) {
			return githubconnection.StartInstallationResponse{}, nil
		},
		completeFn: func(context.Context, string, int64, string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{}, nil
		},
		listFn: func(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error) {
			return githubconnection.ListAccessibleRepositoriesResult{}, nil
		},
		selectFn: func(_ context.Context, _ string, req githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error) {
			if len(req.RepositoryIDs) != 2 {
				t.Fatalf("expected repository ids to be decoded")
			}
			return githubconnection.SelectRepositoriesResponse{
				State:                 githubconnection.StateSyncing,
				SelectedRepositoryIDs: req.RepositoryIDs,
				CreatedRepositoryIDs:  []string{"repo-1", "repo-2"},
				SyncJobIDs:            []string{"job-1"},
			}, nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:          stubHealthChecker{},
		GitHubConnections: NewGitHubConnectionHandler(service),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/github/repositories/select", strings.NewReader(`{"repositoryIds":[1,2],"autoSync":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rec.Code)
	}

	var body DataResponse[githubconnection.SelectRepositoriesResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Data.State != githubconnection.StateSyncing {
		t.Fatalf("expected syncing state, got %q", body.Data.State)
	}
}

func TestGetGitHubConnectionHandlerReturnsNotModifiedWhenETagMatches(t *testing.T) {
	t.Parallel()

	service := stubGitHubConnectionService{
		getFn: func(_ context.Context, organizationID string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{
				OrganizationID: organizationID,
				Provider:       "github",
				State:          githubconnection.StateConnected,
			}, nil
		},
		startFn: func(context.Context, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error) {
			return githubconnection.StartInstallationResponse{}, nil
		},
		completeFn: func(context.Context, string, int64, string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{}, nil
		},
		listFn: func(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error) {
			return githubconnection.ListAccessibleRepositoriesResult{}, nil
		},
		selectFn: func(context.Context, string, githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error) {
			return githubconnection.SelectRepositoriesResponse{}, nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:          stubHealthChecker{},
		GitHubConnections: NewGitHubConnectionHandler(service),
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/github/connection", nil)
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/github/connection", nil)
	req.Header.Set("If-None-Match", firstRec.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("expected status 304, got %d", rec.Code)
	}
}

func TestDisconnectGitHubConnectionHandler(t *testing.T) {
	t.Parallel()

	var disconnectedOrgID string
	service := stubGitHubConnectionService{
		getFn: func(_ context.Context, organizationID string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{
				OrganizationID: organizationID,
				Provider:       "github",
				State:          githubconnection.StateNotConnected,
			}, nil
		},
		startFn: func(context.Context, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error) {
			return githubconnection.StartInstallationResponse{}, nil
		},
		completeFn: func(context.Context, string, int64, string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{}, nil
		},
		listFn: func(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error) {
			return githubconnection.ListAccessibleRepositoriesResult{}, nil
		},
		selectFn: func(context.Context, string, githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error) {
			return githubconnection.SelectRepositoriesResponse{}, nil
		},
		disconnectFn: func(_ context.Context, organizationID string) error {
			disconnectedOrgID = organizationID
			return nil
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:          stubHealthChecker{},
		GitHubConnections: NewGitHubConnectionHandler(service),
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/github/connection", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if disconnectedOrgID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
		t.Fatalf("expected disconnect to be called with organization id, got %q", disconnectedOrgID)
	}

	var body DataResponse[githubconnection.ConnectionResponse]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.State != githubconnection.StateNotConnected {
		t.Fatalf("expected not_connected state, got %q", body.Data.State)
	}
}

func TestDisconnectGitHubConnectionHandlerReturnsNotFound(t *testing.T) {
	t.Parallel()

	service := stubGitHubConnectionService{
		getFn: func(context.Context, string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{}, nil
		},
		startFn: func(context.Context, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error) {
			return githubconnection.StartInstallationResponse{}, nil
		},
		completeFn: func(context.Context, string, int64, string) (githubconnection.ConnectionResponse, error) {
			return githubconnection.ConnectionResponse{}, nil
		},
		listFn: func(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error) {
			return githubconnection.ListAccessibleRepositoriesResult{}, nil
		},
		selectFn: func(context.Context, string, githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error) {
			return githubconnection.SelectRepositoriesResponse{}, nil
		},
		disconnectFn: func(context.Context, string) error {
			return githubconnection.ErrInstallationNotFound
		},
	}

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		Postgres:          stubHealthChecker{},
		GitHubConnections: NewGitHubConnectionHandler(service),
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/github/connection", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}
