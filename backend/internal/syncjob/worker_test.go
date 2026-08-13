package syncjob

import (
	"context"
	"io"
	"log/slog"
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
	})

	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), store, service, 10*time.Millisecond)

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
