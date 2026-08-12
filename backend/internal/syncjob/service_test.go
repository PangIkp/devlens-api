package syncjob

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/githubclient"
)

type stubStore struct {
	ensureRepositoryExistsFn  func(context.Context, string) error
	hasActiveJobFn            func(context.Context, string) (bool, error)
	createFn                  func(context.Context, createParams) (SyncJobResponse, error)
	getByIDFn                 func(context.Context, string) (SyncJobResponse, error)
	listByRepositoryFn        func(context.Context, ListParams) (ListResult, error)
	retryFn                   func(context.Context, string, time.Time) (SyncJobResponse, error)
	cancelFn                  func(context.Context, string, time.Time) (SyncJobResponse, error)
	getRepositoryTargetFn     func(context.Context, string) (repositoryTarget, error)
	markRunningFn             func(context.Context, string, int, time.Time) (SyncJobResponse, error)
	updateProgressFn          func(context.Context, string, int, time.Time) (SyncJobResponse, error)
	markFailedFn              func(context.Context, string, string, time.Time) (SyncJobResponse, error)
	syncRepositoryMetadataFn  func(context.Context, string, repositoryMetadata, time.Time) error
	upsertPullRequestBundleFn func(context.Context, pullRequestInput, []pullRequestReviewInput) error
	replacePullRequestFilesFn func(context.Context, string, int64, []fileChangeInput) error
	upsertWorkflowRunFn       func(context.Context, string, workflowRunInput) error
	upsertDeploymentFn        func(context.Context, string, deploymentInput) error
	completeFn                func(context.Context, string, string, time.Time) (SyncJobResponse, error)
	getCheckpointFn           func(context.Context, string, string, string) (*checkpointRecord, error)
	upsertCheckpointFn        func(context.Context, string, string, string, string, *string, string, *time.Time) error
}

func (s stubStore) EnsureRepositoryExists(ctx context.Context, repositoryID string) error {
	if s.ensureRepositoryExistsFn == nil {
		return nil
	}
	return s.ensureRepositoryExistsFn(ctx, repositoryID)
}

func (s stubStore) HasActiveJob(ctx context.Context, repositoryID string) (bool, error) {
	if s.hasActiveJobFn == nil {
		return false, nil
	}
	return s.hasActiveJobFn(ctx, repositoryID)
}

func (s stubStore) Create(ctx context.Context, params createParams) (SyncJobResponse, error) {
	if s.createFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.createFn(ctx, params)
}

func (s stubStore) GetByID(ctx context.Context, id string) (SyncJobResponse, error) {
	if s.getByIDFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.getByIDFn(ctx, id)
}

func (s stubStore) ListByRepository(ctx context.Context, params ListParams) (ListResult, error) {
	if s.listByRepositoryFn == nil {
		return ListResult{}, nil
	}
	return s.listByRepositoryFn(ctx, params)
}

func (s stubStore) Retry(ctx context.Context, id string, at time.Time) (SyncJobResponse, error) {
	if s.retryFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.retryFn(ctx, id, at)
}

func (s stubStore) Cancel(ctx context.Context, id string, at time.Time) (SyncJobResponse, error) {
	if s.cancelFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.cancelFn(ctx, id, at)
}

func (s stubStore) GetRepositoryTarget(ctx context.Context, repositoryID string) (repositoryTarget, error) {
	if s.getRepositoryTargetFn == nil {
		return repositoryTarget{}, nil
	}
	return s.getRepositoryTargetFn(ctx, repositoryID)
}

func (s stubStore) MarkRunning(ctx context.Context, id string, progress int, at time.Time) (SyncJobResponse, error) {
	if s.markRunningFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.markRunningFn(ctx, id, progress, at)
}

func (s stubStore) UpdateProgress(ctx context.Context, id string, progress int, at time.Time) (SyncJobResponse, error) {
	if s.updateProgressFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.updateProgressFn(ctx, id, progress, at)
}

func (s stubStore) MarkFailed(ctx context.Context, id string, message string, at time.Time) (SyncJobResponse, error) {
	if s.markFailedFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.markFailedFn(ctx, id, message, at)
}

func (s stubStore) SyncRepositoryMetadata(ctx context.Context, repositoryID string, metadata repositoryMetadata, at time.Time) error {
	if s.syncRepositoryMetadataFn == nil {
		return nil
	}
	return s.syncRepositoryMetadataFn(ctx, repositoryID, metadata, at)
}

func (s stubStore) UpsertPullRequestBundle(ctx context.Context, pullRequest pullRequestInput, reviews []pullRequestReviewInput) error {
	if s.upsertPullRequestBundleFn == nil {
		return nil
	}
	return s.upsertPullRequestBundleFn(ctx, pullRequest, reviews)
}

func (s stubStore) ReplacePullRequestFiles(ctx context.Context, repositoryID string, githubPRID int64, files []fileChangeInput) error {
	if s.replacePullRequestFilesFn == nil {
		return nil
	}
	return s.replacePullRequestFilesFn(ctx, repositoryID, githubPRID, files)
}

func (s stubStore) UpsertWorkflowRun(ctx context.Context, repositoryID string, run workflowRunInput) error {
	if s.upsertWorkflowRunFn == nil {
		return nil
	}
	return s.upsertWorkflowRunFn(ctx, repositoryID, run)
}

func (s stubStore) UpsertDeployment(ctx context.Context, repositoryID string, deployment deploymentInput) error {
	if s.upsertDeploymentFn == nil {
		return nil
	}
	return s.upsertDeploymentFn(ctx, repositoryID, deployment)
}

func (s stubStore) Complete(ctx context.Context, id string, repositoryID string, at time.Time) (SyncJobResponse, error) {
	if s.completeFn == nil {
		return SyncJobResponse{}, nil
	}
	return s.completeFn(ctx, id, repositoryID, at)
}

func (s stubStore) GetCheckpoint(ctx context.Context, jobID string, resourceType string, key string) (*checkpointRecord, error) {
	if s.getCheckpointFn == nil {
		return nil, nil
	}
	return s.getCheckpointFn(ctx, jobID, resourceType, key)
}

func (s stubStore) UpsertCheckpoint(ctx context.Context, jobID string, repositoryID string, resourceType string, key string, value *string, status string, lastProcessedAt *time.Time) error {
	if s.upsertCheckpointFn == nil {
		return nil
	}
	return s.upsertCheckpointFn(ctx, jobID, repositoryID, resourceType, key, value, status, lastProcessedAt)
}

type stubGitHubClient struct {
	getRepositoryFn  func(context.Context, string, string) (githubclient.Repository, error)
	getPullRequestFn func(context.Context, string, string, int) (githubclient.PullRequest, error)
	listPullsFn      func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error)
	listReviewsFn    func(context.Context, string, string, int, githubclient.ListOptions) (githubclient.Page[githubclient.Review], error)
	listCommitsFn    func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error)
	listFilesFn      func(context.Context, string, string, int, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequestFile], error)
	listWorkflowFn   func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.WorkflowRun], error)
	listDeployFn     func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.Deployment], error)
	listDepStatusFn  func(context.Context, string, string, int64, githubclient.ListOptions) (githubclient.Page[githubclient.DeploymentStatus], error)
}

func (s stubGitHubClient) GetRepository(ctx context.Context, owner string, repo string) (githubclient.Repository, error) {
	return s.getRepositoryFn(ctx, owner, repo)
}

func (s stubGitHubClient) GetPullRequest(ctx context.Context, owner string, repo string, pullNumber int) (githubclient.PullRequest, error) {
	return s.getPullRequestFn(ctx, owner, repo, pullNumber)
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

func (s stubGitHubClient) ListPullRequestFiles(ctx context.Context, owner string, repo string, pullNumber int, options githubclient.ListOptions) (githubclient.Page[githubclient.PullRequestFile], error) {
	if s.listFilesFn == nil {
		return githubclient.Page[githubclient.PullRequestFile]{}, nil
	}
	return s.listFilesFn(ctx, owner, repo, pullNumber, options)
}

func (s stubGitHubClient) ListWorkflowRuns(ctx context.Context, owner string, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.WorkflowRun], error) {
	if s.listWorkflowFn == nil {
		return githubclient.Page[githubclient.WorkflowRun]{}, nil
	}
	return s.listWorkflowFn(ctx, owner, repo, options)
}

func (s stubGitHubClient) ListDeployments(ctx context.Context, owner string, repo string, options githubclient.ListOptions) (githubclient.Page[githubclient.Deployment], error) {
	if s.listDeployFn == nil {
		return githubclient.Page[githubclient.Deployment]{}, nil
	}
	return s.listDeployFn(ctx, owner, repo, options)
}

func (s stubGitHubClient) ListDeploymentStatuses(ctx context.Context, owner string, repo string, deploymentID int64, options githubclient.ListOptions) (githubclient.Page[githubclient.DeploymentStatus], error) {
	if s.listDepStatusFn == nil {
		return githubclient.Page[githubclient.DeploymentStatus]{}, nil
	}
	return s.listDepStatusFn(ctx, owner, repo, deploymentID, options)
}

func TestServiceCreateRunsManualSyncWithStateAll(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	progressUpdates := 0
	upserted := 0

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
		upsertPullRequestBundleFn: func(_ context.Context, pullRequest pullRequestInput, reviews []pullRequestReviewInput) error {
			upserted++
			if pullRequest.RepositoryID != "repo-1" || pullRequest.GitHubPRID != 1001 || pullRequest.Number != 7 {
				t.Fatalf("unexpected pull request %+v", pullRequest)
			}
			if pullRequest.Additions != 120 || pullRequest.Deletions != 15 || pullRequest.FilesChanged != 4 {
				t.Fatalf("unexpected pull request stats %+v", pullRequest)
			}
			if len(reviews) != 1 || reviews[0].GitHubReviewID != 1 || reviews[0].Reviewer != "bob" {
				t.Fatalf("unexpected reviews %+v", reviews)
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
				Items: []githubclient.PullRequest{{ID: 1001, Number: 7, Title: "Add github sync", UpdatedAt: now, CreatedAt: now, User: githubclient.User{Login: "alice"}}},
			}, nil
		},
		getPullRequestFn: func(_ context.Context, owner string, repo string, pullNumber int) (githubclient.PullRequest, error) {
			if pullNumber != 7 {
				t.Fatalf("unexpected pull number %d", pullNumber)
			}
			return githubclient.PullRequest{
				ID:           1001,
				Number:       7,
				Title:        "Add github sync",
				State:        "closed",
				User:         githubclient.User{Login: "alice"},
				CreatedAt:    now,
				UpdatedAt:    now,
				Additions:    120,
				Deletions:    15,
				ChangedFiles: 4,
			}, nil
		},
		listReviewsFn: func(_ context.Context, owner string, repo string, pullNumber int, options githubclient.ListOptions) (githubclient.Page[githubclient.Review], error) {
			if pullNumber != 7 || options.PerPage != 100 {
				t.Fatalf("unexpected review request %+v %d", options, pullNumber)
			}
			return githubclient.Page[githubclient.Review]{Items: []githubclient.Review{{ID: 1, State: "APPROVED", User: githubclient.User{Login: "bob"}, SubmittedAt: &now}}}, nil
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
	if upserted != 1 {
		t.Fatalf("expected one upsert, got %d", upserted)
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
		syncRepositoryMetadataFn:  func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		upsertPullRequestBundleFn: func(context.Context, pullRequestInput, []pullRequestReviewInput) error { return nil },
		completeFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{}, nil
		},
	}, stubGitHubClient{})

	_, err := svc.Create(context.Background(), "repo-1", CreateSyncRequest{})
	if !errors.Is(err, ErrSyncJobConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestProcessPendingUsesStoredFullSyncOptions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)

	svc := NewService(stubStore{
		getByIDFn: func(context.Context, string) (SyncJobResponse, error) {
			return SyncJobResponse{ID: "job-1", RepositoryID: "repo-1", Status: StatusPending, CreatedAt: now}, nil
		},
		getCheckpointFn: func(_ context.Context, jobID string, resourceType string, key string) (*checkpointRecord, error) {
			if jobID != "job-1" {
				t.Fatalf("unexpected job id %q", jobID)
			}
			if resourceType == "job" && key == "mode" {
				value := ModeFull
				return &checkpointRecord{Value: &value, Status: "pending"}, nil
			}
			return nil, nil
		},
		markRunningFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: now}, nil
		},
		getRepositoryTargetFn: func(context.Context, string) (repositoryTarget, error) {
			return repositoryTarget{ID: "repo-1", FullName: "devlens-labs/devlens-api"}, nil
		},
		syncRepositoryMetadataFn:  func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		upsertPullRequestBundleFn: func(context.Context, pullRequestInput, []pullRequestReviewInput) error { return nil },
		updateProgressFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: now}, nil
		},
		completeFn: func(_ context.Context, id string, repositoryID string, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: repositoryID, Status: StatusCompleted, Progress: 100, CreatedAt: now}, nil
		},
	}, stubGitHubClient{
		getRepositoryFn: func(context.Context, string, string) (githubclient.Repository, error) {
			return githubclient.Repository{Name: "devlens-api", FullName: "devlens-labs/devlens-api", DefaultBranch: "main"}, nil
		},
		listPullsFn: func(_ context.Context, _ string, _ string, options githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
			if options.State != "all" {
				t.Fatalf("expected full sync state=all, got %q", options.State)
			}
			return githubclient.Page[githubclient.PullRequest]{}, nil
		},
		getPullRequestFn: func(context.Context, string, string, int) (githubclient.PullRequest, error) {
			return githubclient.PullRequest{}, nil
		},
		listReviewsFn: func(context.Context, string, string, int, githubclient.ListOptions) (githubclient.Page[githubclient.Review], error) {
			return githubclient.Page[githubclient.Review]{}, nil
		},
		listCommitsFn: func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error) {
			return githubclient.Page[githubclient.Commit]{}, nil
		},
	})
	svc.now = func() time.Time { return now }

	result, err := svc.ProcessPending(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestRetryOnlyAllowsFailedJobs(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		getByIDFn: func(context.Context, string) (SyncJobResponse, error) {
			return SyncJobResponse{ID: "job-1", RepositoryID: "repo-1", Status: StatusCompleted, CreatedAt: time.Now().UTC()}, nil
		},
	}, stubGitHubClient{})

	_, err := svc.Retry(context.Background(), "job-1")
	if !errors.Is(err, ErrSyncJobRetryState) {
		t.Fatalf("expected retry state error, got %v", err)
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
		syncRepositoryMetadataFn:  func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		upsertPullRequestBundleFn: func(context.Context, pullRequestInput, []pullRequestReviewInput) error { return nil },
		completeFn: func(context.Context, string, string, time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{}, nil
		},
	}, stubGitHubClient{
		getRepositoryFn: func(context.Context, string, string) (githubclient.Repository, error) {
			return githubclient.Repository{}, errors.New("upstream unavailable")
		},
		getPullRequestFn: func(context.Context, string, string, int) (githubclient.PullRequest, error) {
			return githubclient.PullRequest{}, nil
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
		syncRepositoryMetadataFn:  func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		upsertPullRequestBundleFn: func(context.Context, pullRequestInput, []pullRequestReviewInput) error { return nil },
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
