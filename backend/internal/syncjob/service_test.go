package syncjob

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/githubclient"
)

type stubStore struct {
	ensureRepositoryExistsFn func(context.Context, string) error
	hasActiveJobFn           func(context.Context, string) (bool, error)
	createFn                 func(context.Context, createParams) (SyncJobResponse, error)
	getByIDFn                func(context.Context, string) (SyncJobResponse, error)
	listByRepositoryFn       func(context.Context, ListParams) (ListResult, error)
	getRepositoryTargetFn    func(context.Context, string) (repositoryTarget, error)
	markRunningFn            func(context.Context, string, int, time.Time) (SyncJobResponse, error)
	updateProgressFn         func(context.Context, string, int, time.Time) (SyncJobResponse, error)
	markFailedFn             func(context.Context, string, string, time.Time) (SyncJobResponse, error)
	syncRepositoryMetadataFn func(context.Context, string, repositoryMetadata, time.Time) error
	completeFn               func(context.Context, string, string, time.Time) (SyncJobResponse, error)
}

func (s stubStore) EnsureRepositoryExists(ctx context.Context, repositoryID string) error {
	return s.ensureRepositoryExistsFn(ctx, repositoryID)
}

func (s stubStore) HasActiveJob(ctx context.Context, repositoryID string) (bool, error) {
	return s.hasActiveJobFn(ctx, repositoryID)
}

func (s stubStore) Create(ctx context.Context, params createParams) (SyncJobResponse, error) {
	return s.createFn(ctx, params)
}

func (s stubStore) GetByID(ctx context.Context, id string) (SyncJobResponse, error) {
	return s.getByIDFn(ctx, id)
}

func (s stubStore) ListByRepository(ctx context.Context, params ListParams) (ListResult, error) {
	return s.listByRepositoryFn(ctx, params)
}

func (s stubStore) GetRepositoryTarget(ctx context.Context, repositoryID string) (repositoryTarget, error) {
	return s.getRepositoryTargetFn(ctx, repositoryID)
}

func (s stubStore) MarkRunning(ctx context.Context, id string, progress int, at time.Time) (SyncJobResponse, error) {
	return s.markRunningFn(ctx, id, progress, at)
}

func (s stubStore) UpdateProgress(ctx context.Context, id string, progress int, at time.Time) (SyncJobResponse, error) {
	return s.updateProgressFn(ctx, id, progress, at)
}

func (s stubStore) MarkFailed(ctx context.Context, id string, message string, at time.Time) (SyncJobResponse, error) {
	return s.markFailedFn(ctx, id, message, at)
}

func (s stubStore) SyncRepositoryMetadata(ctx context.Context, repositoryID string, metadata repositoryMetadata, at time.Time) error {
	return s.syncRepositoryMetadataFn(ctx, repositoryID, metadata, at)
}

func (s stubStore) Complete(ctx context.Context, id string, repositoryID string, at time.Time) (SyncJobResponse, error) {
	return s.completeFn(ctx, id, repositoryID, at)
}

type stubGitHubClient struct {
	getRepositoryFn func(context.Context, string, string) (githubclient.Repository, error)
	listPullsFn     func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error)
	listReviewsFn   func(context.Context, string, string, int, githubclient.ListOptions) (githubclient.Page[githubclient.Review], error)
	listCommitsFn   func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error)
}

func (s stubGitHubClient) GetRepository(ctx context.Context, owner string, repo string) (githubclient.Repository, error) {
	return s.getRepositoryFn(ctx, owner, repo)
}

func (s stubGitHubClient) ListPullRequests(ctx context.Context, owner string, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
	return s.listPullsFn(ctx, owner, repo, options)
}

func (s stubGitHubClient) ListReviews(ctx context.Context, owner string, repo string, pullNumber int, options githubclient.ListOptions) (githubclient.Page[githubclient.Review], error) {
	return s.listReviewsFn(ctx, owner, repo, pullNumber, options)
}

func (s stubGitHubClient) ListCommits(ctx context.Context, owner string, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error) {
	return s.listCommitsFn(ctx, owner, repo, options)
}

func TestServiceCreateRunsManualSyncWithStateAll(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	progressUpdates := 0

	svc := NewService(stubStore{
		ensureRepositoryExistsFn: func(_ context.Context, repositoryID string) error {
			if repositoryID != "repo-1" {
				t.Fatalf("unexpected repository id %q", repositoryID)
			}
			return nil
		},
		hasActiveJobFn: func(context.Context, string) (bool, error) { return false, nil },
		createFn: func(_ context.Context, params createParams) (SyncJobResponse, error) {
			return SyncJobResponse{ID: "job-1", RepositoryID: params.RepositoryID, Status: StatusPending, Progress: 0, CreatedAt: now}, nil
		},
		getByIDFn:          func(context.Context, string) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		listByRepositoryFn: func(context.Context, ListParams) (ListResult, error) { return ListResult{}, nil },
		getRepositoryTargetFn: func(context.Context, string) (repositoryTarget, error) {
			return repositoryTarget{ID: "repo-1", FullName: "devlens-labs/devlens-api"}, nil
		},
		markRunningFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			if progress != 5 {
				t.Fatalf("unexpected running progress %d", progress)
			}
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: now}, nil
		},
		updateProgressFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			progressUpdates++
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: now}, nil
		},
		markFailedFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			t.Fatal("markFailed should not be called")
			return SyncJobResponse{}, nil
		},
		syncRepositoryMetadataFn: func(_ context.Context, repositoryID string, metadata repositoryMetadata, _ time.Time) error {
			if repositoryID != "repo-1" || metadata.FullName != "devlens-labs/devlens-api" || metadata.IsActive != true {
				t.Fatalf("unexpected metadata %+v", metadata)
			}
			return nil
		},
		completeFn: func(_ context.Context, id string, repositoryID string, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: repositoryID, Status: StatusCompleted, Progress: 100, CreatedAt: now}, nil
		},
	}, stubGitHubClient{
		getRepositoryFn: func(_ context.Context, owner string, repo string) (githubclient.Repository, error) {
			if owner != "devlens-labs" || repo != "devlens-api" {
				t.Fatalf("unexpected target %s/%s", owner, repo)
			}
			return githubclient.Repository{Name: "devlens-api", FullName: "devlens-labs/devlens-api", DefaultBranch: "main"}, nil
		},
		listPullsFn: func(_ context.Context, owner string, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
			if options.State != "all" {
				t.Fatalf("expected state=all, got %q", options.State)
			}
			return githubclient.Page[githubclient.PullRequest]{
				Items: []githubclient.PullRequest{{Number: 7, UpdatedAt: now}},
			}, nil
		},
		listReviewsFn: func(_ context.Context, owner string, repo string, pullNumber int, options githubclient.ListOptions) (githubclient.Page[githubclient.Review], error) {
			if pullNumber != 7 || options.PerPage != 100 {
				t.Fatalf("unexpected review request %+v %d", options, pullNumber)
			}
			return githubclient.Page[githubclient.Review]{Items: []githubclient.Review{{ID: 1}}}, nil
		},
		listCommitsFn: func(_ context.Context, owner string, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error) {
			return githubclient.Page[githubclient.Commit]{
				Items: []githubclient.Commit{{SHA: "abc", Commit: githubclient.CommitDetail{Author: githubclient.CommitAuthor{Date: now}}}},
			}, nil
		},
	})
	svc.now = func() time.Time { return now }

	result, err := svc.Create(context.Background(), "repo-1", CreateSyncRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("unexpected result %+v", result)
	}
	if progressUpdates < 2 {
		t.Fatalf("expected progress updates, got %d", progressUpdates)
	}
}

func TestServiceCreateConflictWhenActiveJobExists(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		ensureRepositoryExistsFn: func(context.Context, string) error { return nil },
		hasActiveJobFn:           func(context.Context, string) (bool, error) { return true, nil },
		createFn: func(context.Context, createParams) (SyncJobResponse, error) {
			t.Fatal("create should not be called")
			return SyncJobResponse{}, nil
		},
		getByIDFn:             func(context.Context, string) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		listByRepositoryFn:    func(context.Context, ListParams) (ListResult, error) { return ListResult{}, nil },
		getRepositoryTargetFn: func(context.Context, string) (repositoryTarget, error) { return repositoryTarget{}, nil },
		markRunningFn:         func(context.Context, string, int, time.Time) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		updateProgressFn:      func(context.Context, string, int, time.Time) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		markFailedFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{}, nil
		},
		syncRepositoryMetadataFn: func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		completeFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{}, nil
		},
	}, stubGitHubClient{})

	_, err := svc.Create(context.Background(), "repo-1", CreateSyncRequest{})
	if !errors.Is(err, ErrSyncJobConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestServiceCreateMarksFailedWhenGitHubErrors(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	var failedMessage string

	svc := NewService(stubStore{
		ensureRepositoryExistsFn: func(context.Context, string) error { return nil },
		hasActiveJobFn:           func(context.Context, string) (bool, error) { return false, nil },
		createFn: func(context.Context, createParams) (SyncJobResponse, error) {
			return SyncJobResponse{ID: "job-1", RepositoryID: "repo-1", CreatedAt: now}, nil
		},
		getByIDFn:          func(context.Context, string) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		listByRepositoryFn: func(context.Context, ListParams) (ListResult, error) { return ListResult{}, nil },
		getRepositoryTargetFn: func(context.Context, string) (repositoryTarget, error) {
			return repositoryTarget{ID: "repo-1", FullName: "devlens-labs/devlens-api"}, nil
		},
		markRunningFn: func(context.Context, string, int, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: "job-1", RepositoryID: "repo-1", Status: StatusRunning, CreatedAt: now}, nil
		},
		updateProgressFn: func(context.Context, string, int, time.Time) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		markFailedFn: func(_ context.Context, id string, message string, _ time.Time) (SyncJobResponse, error) {
			failedMessage = message
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusFailed, CreatedAt: now}, nil
		},
		syncRepositoryMetadataFn: func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		completeFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{}, nil
		},
	}, stubGitHubClient{
		getRepositoryFn: func(context.Context, string, string) (githubclient.Repository, error) {
			return githubclient.Repository{}, errors.New("upstream unavailable")
		},
		listPullsFn: func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
			return githubclient.Page[githubclient.PullRequest]{}, nil
		},
		listReviewsFn: func(context.Context, string, string, int, githubclient.ListOptions) (githubclient.Page[githubclient.Review], error) {
			return githubclient.Page[githubclient.Review]{}, nil
		},
		listCommitsFn: func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error) {
			return githubclient.Page[githubclient.Commit]{}, nil
		},
	})
	svc.now = func() time.Time { return now }

	result, err := svc.Create(context.Background(), "repo-1", CreateSyncRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != StatusFailed {
		t.Fatalf("expected failed job, got %+v", result)
	}
	if failedMessage == "" {
		t.Fatal("expected failed message to be recorded")
	}
}

func TestServiceListValidation(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		ensureRepositoryExistsFn: func(context.Context, string) error { return nil },
		hasActiveJobFn:           func(context.Context, string) (bool, error) { return false, nil },
		createFn:                 func(context.Context, createParams) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		getByIDFn:                func(context.Context, string) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		listByRepositoryFn: func(context.Context, ListParams) (ListResult, error) {
			t.Fatal("list should not be called")
			return ListResult{}, nil
		},
		getRepositoryTargetFn: func(context.Context, string) (repositoryTarget, error) { return repositoryTarget{}, nil },
		markRunningFn:         func(context.Context, string, int, time.Time) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		updateProgressFn:      func(context.Context, string, int, time.Time) (SyncJobResponse, error) { return SyncJobResponse{}, nil },
		markFailedFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{}, nil
		},
		syncRepositoryMetadataFn: func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		completeFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{}, nil
		},
	}, stubGitHubClient{})

	_, err := svc.ListByRepository(context.Background(), ListParams{
		RepositoryID: "repo-1",
		Page:         1,
		PageSize:     20,
		Status:       "broken",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
