package syncjob

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"sync"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/githubclient"
)

type stubPendingJobStore struct {
	mu    sync.Mutex
	calls []int
	ids   []string
}

func (s *stubPendingJobStore) ListPendingIDs(_ context.Context, limit int) ([]string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, limit)
	s.mu.Unlock()
	return append([]string(nil), s.ids...), nil
}

func TestWorkerProcessesPendingJobsImmediatelyAndOnInterval(t *testing.T) {
	t.Parallel()

	store := &stubPendingJobStore{ids: []string{"job-1"}}
	processed := make(chan string, 4)

	service := NewService(stubStore{
		getByIDFn: func(_ context.Context, id string) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusPending, CreatedAt: time.Now().UTC()}, nil
		},
		markRunningFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			processed <- id + ":running"
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: time.Now().UTC()}, nil
		},
		getRepositoryTargetFn: func(context.Context, string) (repositoryTarget, error) {
			return repositoryTarget{ID: "repo-1", FullName: "devlens-labs/devlens-api"}, nil
		},
		syncRepositoryMetadataFn:  func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		upsertPullRequestBundleFn: func(context.Context, pullRequestInput, []pullRequestReviewInput) error { return nil },
		updateProgressFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: time.Now().UTC()}, nil
		},
		completeFn: func(_ context.Context, id string, repositoryID string, _ time.Time) (SyncJobResponse, error) {
			processed <- id + ":completed"
			return SyncJobResponse{ID: id, RepositoryID: repositoryID, Status: StatusCompleted, Progress: 100, CreatedAt: time.Now().UTC()}, nil
		},
	}, stubGitHubClient{
		getRepositoryFn: func(context.Context, string, string) (githubclient.Repository, error) {
			return githubclient.Repository{Name: "devlens-api", FullName: "devlens-labs/devlens-api", DefaultBranch: "main"}, nil
		},
		listPullsFn: func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
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
	}, nil)

	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), store, service, 10*time.Millisecond, 10, 2, time.Second, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-processed:
	case <-time.After(time.Second):
		t.Fatal("expected immediate pending job processing")
	}

	select {
	case <-processed:
	case <-time.After(time.Second):
		t.Fatal("expected job completion event")
	}

	select {
	case <-processed:
	case <-time.After(time.Second):
		t.Fatal("expected next interval processing")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected worker shutdown after context cancel")
	}
}

func TestWorkerHonorsBatchSizeAndConcurrency(t *testing.T) {
	t.Parallel()

	store := &stubPendingJobStore{ids: []string{"job-1", "job-2", "job-3"}}
	var active int32
	var maxActive int32

	service := NewService(stubStore{
		getByIDFn: func(_ context.Context, id string) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusPending, CreatedAt: time.Now().UTC()}, nil
		},
		markRunningFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			current := atomic.AddInt32(&active, 1)
			for {
				seen := atomic.LoadInt32(&maxActive)
				if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusCompleted, Progress: progress, CreatedAt: time.Now().UTC()}, nil
		},
		getRepositoryTargetFn: func(context.Context, string) (repositoryTarget, error) {
			return repositoryTarget{ID: "repo-1", FullName: "devlens-labs/devlens-api"}, nil
		},
		syncRepositoryMetadataFn:  func(context.Context, string, repositoryMetadata, time.Time) error { return nil },
		upsertPullRequestBundleFn: func(context.Context, pullRequestInput, []pullRequestReviewInput) error { return nil },
		updateProgressFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: time.Now().UTC()}, nil
		},
		completeFn: func(_ context.Context, id string, repositoryID string, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: repositoryID, Status: StatusCompleted, Progress: 100, CreatedAt: time.Now().UTC()}, nil
		},
	}, stubGitHubClient{
		getRepositoryFn: func(context.Context, string, string) (githubclient.Repository, error) {
			return githubclient.Repository{Name: "devlens-api", FullName: "devlens-labs/devlens-api", DefaultBranch: "main"}, nil
		},
		listPullsFn: func(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
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
	}, nil)

	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), store, service, time.Second, 2, 2, time.Second, nil)
	if err := worker.processOnce(context.Background()); err != nil {
		t.Fatalf("process once: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.calls) == 0 || store.calls[0] != 2 {
		t.Fatalf("expected batch size 2, got %#v", store.calls)
	}
	if maxActive > 2 {
		t.Fatalf("expected concurrency <= 2, got %d", maxActive)
	}
}

func TestWorkerAppliesJobTimeout(t *testing.T) {
	t.Parallel()

	store := &stubPendingJobStore{ids: []string{"job-timeout"}}
	service := NewService(stubStore{
		getByIDFn: func(_ context.Context, id string) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusPending, CreatedAt: time.Now().UTC()}, nil
		},
		markRunningFn: func(_ context.Context, id string, progress int, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusRunning, Progress: progress, CreatedAt: time.Now().UTC()}, nil
		},
		getRepositoryTargetFn: func(ctx context.Context, _ string) (repositoryTarget, error) {
			<-ctx.Done()
			return repositoryTarget{}, ctx.Err()
		},
		markFailedFn: func(_ context.Context, id string, message string, _ time.Time) (SyncJobResponse, error) {
			return SyncJobResponse{ID: id, RepositoryID: "repo-1", Status: StatusFailed, ErrorMessage: &message, CreatedAt: time.Now().UTC()}, nil
		},
	}, stubGitHubClient{}, nil)

	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), store, service, time.Second, 1, 1, 10*time.Millisecond, nil)
	if err := worker.processOnce(context.Background()); err != nil {
		t.Fatalf("process once: %v", err)
	}
}
