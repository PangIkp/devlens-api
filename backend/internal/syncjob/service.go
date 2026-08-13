package syncjob

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/githubclient"
	"github.com/PangIkp/devlens/backend/internal/observability"
)

type store interface {
	EnsureRepositoryExists(context.Context, string) error
	HasActiveJob(context.Context, string) (bool, error)
	Create(context.Context, createParams) (SyncJobResponse, error)
	GetByID(context.Context, string) (SyncJobResponse, error)
	GetRepositoryOrganizationID(context.Context, string) (string, error)
	ListByRepository(context.Context, ListParams) (ListResult, error)
	Retry(context.Context, string, time.Time) (SyncJobResponse, error)
	Cancel(context.Context, string, time.Time) (SyncJobResponse, error)
	GetRepositoryTarget(context.Context, string) (repositoryTarget, error)
	MarkRunning(context.Context, string, int, time.Time) (SyncJobResponse, error)
	UpdateProgress(context.Context, string, int, time.Time) (SyncJobResponse, error)
	MarkFailed(context.Context, string, string, time.Time) (SyncJobResponse, error)
	SyncRepositoryMetadata(context.Context, string, repositoryMetadata, time.Time) error
	UpsertPullRequestBundle(context.Context, pullRequestInput, []pullRequestReviewInput) error
	ReplacePullRequestFiles(context.Context, string, int64, []fileChangeInput) error
	UpsertCommitEvents(context.Context, string, []commitEventInput) error
	UpsertWorkflowRuns(context.Context, string, []workflowRunInput) error
	UpsertDeployments(context.Context, string, []deploymentInput) error
	Complete(context.Context, string, string, time.Time) (SyncJobResponse, error)
	GetCheckpoint(context.Context, string, string, string) (*checkpointRecord, error)
	UpsertCheckpoint(context.Context, string, string, string, string, *string, string, *time.Time) error
}

type Service struct {
	store                 store
	githubClient          githubclient.Client
	publisher             completionPublisher
	now                   func() time.Time
	metrics               *observability.Metrics
	sleep                 func(context.Context, time.Duration) error
	minRateLimitRemaining int
}

var errSyncCanceled = errors.New("sync job canceled")

type SyncCompletedEvent struct {
	OrganizationID string    `json:"organizationId,omitempty"`
	RepositoryID   string    `json:"repositoryId"`
	SyncJobID      string    `json:"syncJobId"`
	OccurredAt     time.Time `json:"occurredAt"`
	EventType      string    `json:"eventType"`
	From           time.Time `json:"from,omitempty"`
	To             time.Time `json:"to,omitempty"`
}

type completionPublisher interface {
	PublishRepositorySyncCompleted(context.Context, SyncCompletedEvent) error
}

type CompositePublisher struct {
	publishers []completionPublisher
}

func NewCompositePublisher(publishers ...completionPublisher) *CompositePublisher {
	items := make([]completionPublisher, 0, len(publishers))
	for _, publisher := range publishers {
		if publisher != nil {
			items = append(items, publisher)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &CompositePublisher{publishers: items}
}

func (p *CompositePublisher) PublishRepositorySyncCompleted(ctx context.Context, event SyncCompletedEvent) error {
	if p == nil {
		return nil
	}
	for _, publisher := range p.publishers {
		if err := publisher.PublishRepositorySyncCompleted(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func NewService(store store, githubClient githubclient.Client, metrics *observability.Metrics) *Service {
	return &Service{
		store:        store,
		githubClient: githubClient,
		now:          time.Now,
		metrics:      metrics,
		sleep:        sleepWithContext,
	}
}

func (s *Service) ConfigureRateLimitThrottle(minRemaining int) {
	if minRemaining >= 0 {
		s.minRateLimitRemaining = minRemaining
	}
}

func (s *Service) SetCompletionPublisher(publisher completionPublisher) {
	s.publisher = publisher
}

func (s *Service) Create(ctx context.Context, repositoryID string, req CreateSyncRequest) (SyncJobResponse, error) {
	options, err := validateCreateRequest(req)
	if err != nil {
		return SyncJobResponse{}, err
	}

	return s.createAndRun(ctx, repositoryID, options)
}

func (s *Service) Enqueue(ctx context.Context, repositoryID string, req CreateSyncRequest) (SyncJobResponse, error) {
	options, err := validateCreateRequest(req)
	if err != nil {
		return SyncJobResponse{}, err
	}

	_ = options

	if err := s.store.EnsureRepositoryExists(ctx, repositoryID); err != nil {
		return SyncJobResponse{}, err
	}

	active, err := s.store.HasActiveJob(ctx, repositoryID)
	if err != nil {
		return SyncJobResponse{}, err
	}
	if active {
		return SyncJobResponse{}, ErrSyncJobConflict
	}

	job, err := s.store.Create(ctx, createParams{RepositoryID: repositoryID, IdempotencyKey: stringPtr(req.IdempotencyKey)})
	if err != nil {
		return SyncJobResponse{}, err
	}
	if err := s.persistSyncOptions(ctx, job.ID, repositoryID, options); err != nil {
		return SyncJobResponse{}, err
	}

	return job, nil
}

func (s *Service) createAndRun(ctx context.Context, repositoryID string, options syncOptions) (SyncJobResponse, error) {
	if err := s.store.EnsureRepositoryExists(ctx, repositoryID); err != nil {
		return SyncJobResponse{}, err
	}

	active, err := s.store.HasActiveJob(ctx, repositoryID)
	if err != nil {
		return SyncJobResponse{}, err
	}
	if active {
		return SyncJobResponse{}, ErrSyncJobConflict
	}

	job, err := s.store.Create(ctx, createParams{RepositoryID: repositoryID, IdempotencyKey: stringPtr(reqIdempotencyKey(options))})
	if err != nil {
		return SyncJobResponse{}, err
	}
	if err := s.persistSyncOptions(ctx, job.ID, repositoryID, options); err != nil {
		return SyncJobResponse{}, err
	}

	return s.run(ctx, job, options)
}

func (s *Service) ProcessPending(ctx context.Context, id string) (SyncJobResponse, error) {
	job, err := s.store.GetByID(ctx, id)
	if err != nil {
		return SyncJobResponse{}, err
	}
	if job.Status != StatusPending {
		return job, nil
	}
	options, err := s.loadSyncOptions(ctx, job.ID)
	if err != nil {
		return SyncJobResponse{}, err
	}
	return s.run(ctx, job, options)
}

func (s *Service) Retry(ctx context.Context, id string) (SyncJobResponse, error) {
	job, err := s.store.GetByID(ctx, id)
	if err != nil {
		return SyncJobResponse{}, err
	}
	if job.Status != StatusFailed {
		return SyncJobResponse{}, ErrSyncJobRetryState
	}

	active, err := s.store.HasActiveJob(ctx, job.RepositoryID)
	if err != nil {
		return SyncJobResponse{}, err
	}
	if active {
		return SyncJobResponse{}, ErrSyncJobConflict
	}

	return s.store.Retry(ctx, id, s.now().UTC())
}

func (s *Service) Cancel(ctx context.Context, id string) (SyncJobResponse, error) {
	job, err := s.store.GetByID(ctx, id)
	if err != nil {
		return SyncJobResponse{}, err
	}
	if job.Status != StatusPending && job.Status != StatusRunning {
		return SyncJobResponse{}, ErrSyncJobCancelState
	}
	return s.store.Cancel(ctx, id, s.now().UTC())
}

func (s *Service) GetByID(ctx context.Context, id string) (SyncJobResponse, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) ListByRepository(ctx context.Context, params ListParams) (ListResult, error) {
	if err := validateListParams(params); err != nil {
		return ListResult{}, err
	}
	if err := s.store.EnsureRepositoryExists(ctx, params.RepositoryID); err != nil {
		return ListResult{}, err
	}
	if params.SortOrder == "" {
		params.SortOrder = "desc"
	}
	return s.store.ListByRepository(ctx, params)
}

type syncOptions struct {
	mode           string
	from           *time.Time
	idempotencyKey string
}

func validateCreateRequest(req CreateSyncRequest) (syncOptions, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = ModeIncremental
	}
	if mode != ModeIncremental && mode != ModeFull {
		return syncOptions{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "mode", Message: "must be one of incremental, full"}},
		}
	}

	var from *time.Time
	if req.From != nil && strings.TrimSpace(*req.From) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*req.From))
		if err != nil {
			return syncOptions{}, &ValidationError{
				Message: "request validation failed",
				Details: []ValidationIssue{{Field: "from", Message: "must be a valid date in YYYY-MM-DD format"}},
			}
		}
		utc := parsed.UTC()
		from = &utc
	}

	return syncOptions{mode: mode, from: from, idempotencyKey: strings.TrimSpace(req.IdempotencyKey)}, nil
}

func validateListParams(params ListParams) error {
	var issues []ValidationIssue

	if params.Status != "" &&
		params.Status != StatusPending &&
		params.Status != StatusRunning &&
		params.Status != StatusCompleted &&
		params.Status != StatusFailed &&
		params.Status != StatusCanceled {
		issues = append(issues, ValidationIssue{Field: "status", Message: "must be one of pending, running, completed, failed, canceled"})
	}

	if params.SortOrder != "" && params.SortOrder != "asc" && params.SortOrder != "desc" {
		issues = append(issues, ValidationIssue{Field: "sortOrder", Message: "must be one of asc, desc"})
	}

	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func (s *Service) run(ctx context.Context, job SyncJobResponse, options syncOptions) (SyncJobResponse, error) {
	runStarted := time.Now()
	if err := s.ensureNotCanceled(ctx, job.ID); err != nil {
		s.recordSyncDuration(options.mode, "canceled", time.Since(runStarted))
		return s.canceledJobResult(ctx, job.ID, err)
	}

	startedAt := s.now().UTC()
	job, err := s.store.MarkRunning(ctx, job.ID, 5, startedAt)
	if err != nil {
		return SyncJobResponse{}, err
	}

	target, err := s.store.GetRepositoryTarget(ctx, job.RepositoryID)
	if err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, err)
	}

	owner, repoName, err := splitFullName(target.FullName)
	if err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, err)
	}

	cutoff := resolveSyncCutoff(target, options)

	if err := s.ensureNotCanceled(ctx, job.ID); err != nil {
		s.recordSyncDuration(options.mode, "canceled", time.Since(runStarted))
		return s.canceledJobResult(ctx, job.ID, err)
	}
	repo, err := s.githubClient.GetRepository(ctx, owner, repoName)
	if err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, fmt.Errorf("sync repository metadata: %w", err))
	}

	if err := s.store.SyncRepositoryMetadata(ctx, job.RepositoryID, repositoryMetadata{
		Name:          repo.Name,
		FullName:      repo.FullName,
		DefaultBranch: stringPtr(repo.DefaultBranch),
		IsActive:      !repo.Archived,
		ArchivedAt:    archivedAt(repo.Archived, startedAt),
	}, startedAt); err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, err)
	}

	job, err = s.store.UpdateProgress(ctx, job.ID, 25, s.now().UTC())
	if err != nil {
		return SyncJobResponse{}, err
	}

	if err := s.ensureNotCanceled(ctx, job.ID); err != nil {
		s.recordSyncDuration(options.mode, "canceled", time.Since(runStarted))
		return s.canceledJobResult(ctx, job.ID, err)
	}
	pullRequests, err := s.syncPullRequests(ctx, job.ID, job.RepositoryID, owner, repoName, cutoff)
	if err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, err)
	}

	if pullRequests > 0 {
		job, err = s.store.UpdateProgress(ctx, job.ID, 75, s.now().UTC())
	} else {
		job, err = s.store.UpdateProgress(ctx, job.ID, 60, s.now().UTC())
	}
	if err != nil {
		return SyncJobResponse{}, err
	}

	if err := s.ensureNotCanceled(ctx, job.ID); err != nil {
		s.recordSyncDuration(options.mode, "canceled", time.Since(runStarted))
		return s.canceledJobResult(ctx, job.ID, err)
	}
	if _, err := s.syncCommits(ctx, job.ID, job.RepositoryID, owner, repoName, cutoff); err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, err)
	}

	job, err = s.store.UpdateProgress(ctx, job.ID, 85, s.now().UTC())
	if err != nil {
		return SyncJobResponse{}, err
	}

	if _, err := s.syncWorkflowRuns(ctx, job.ID, job.RepositoryID, owner, repoName, cutoff); err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, err)
	}

	job, err = s.store.UpdateProgress(ctx, job.ID, 92, s.now().UTC())
	if err != nil {
		return SyncJobResponse{}, err
	}

	if _, err := s.syncDeployments(ctx, job.ID, job.RepositoryID, owner, repoName, cutoff); err != nil {
		return s.failJob(ctx, job, options.mode, runStarted, err)
	}

	completedAt := s.now().UTC()
	if s.publisher != nil {
		metricsFrom := time.Time{}
		if cutoff != nil {
			metricsFrom = cutoff.UTC()
		}
		organizationID, err := s.store.GetRepositoryOrganizationID(ctx, job.RepositoryID)
		if err != nil {
			return s.failJob(ctx, job, options.mode, runStarted, fmt.Errorf("load repository organization: %w", err))
		}
		if err := s.publisher.PublishRepositorySyncCompleted(ctx, SyncCompletedEvent{
			EventType:      "repository.sync.completed",
			OrganizationID: organizationID,
			RepositoryID:   job.RepositoryID,
			SyncJobID:      job.ID,
			OccurredAt:     completedAt,
			From:           metricsFrom,
			To:             completedAt,
		}); err != nil {
			return s.failJob(ctx, job, options.mode, runStarted, fmt.Errorf("trigger metrics calculation: %w", err))
		}
	}

	completedJob, err := s.store.Complete(ctx, job.ID, job.RepositoryID, completedAt)
	if err == nil {
		s.recordSyncDuration(options.mode, "completed", time.Since(runStarted))
	}
	return completedJob, err
}

func (s *Service) failJob(ctx context.Context, job SyncJobResponse, mode string, runStarted time.Time, err error) (SyncJobResponse, error) {
	failedAt := s.now().UTC()
	s.recordSyncDuration(mode, "failed", time.Since(runStarted))
	failedJob, markErr := s.store.MarkFailed(ctx, job.ID, err.Error(), failedAt)
	if markErr != nil {
		return SyncJobResponse{}, markErr
	}
	return failedJob, nil
}

func (s *Service) recordSyncDuration(mode, result string, duration time.Duration) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.RecordSyncDuration(mode, result, duration)
}

func (s *Service) syncPullRequests(ctx context.Context, jobID string, repositoryID string, owner string, repoName string, cutoff *time.Time) (int, error) {
	total := 0
	page, err := s.resumePage(ctx, jobID, "pull_requests", "page")
	if err != nil {
		return 0, err
	}

	for {
		if err := s.ensureNotCanceled(ctx, jobID); err != nil {
			return 0, err
		}
		result, err := s.githubClient.ListPullRequests(ctx, owner, repoName, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
			State:   "all",
		})
		if err != nil {
			return 0, fmt.Errorf("list pull requests: %w", err)
		}
		if err := s.maybeThrottleGitHub(ctx, result.RateLimit); err != nil {
			return 0, err
		}

		for _, pullRequest := range result.Items {
			if err := s.ensureNotCanceled(ctx, jobID); err != nil {
				return 0, err
			}
			if !includePullRequest(pullRequest, cutoff) {
				continue
			}

			detail, err := s.githubClient.GetPullRequest(ctx, owner, repoName, pullRequest.Number)
			if err != nil {
				return 0, fmt.Errorf("get pull request %d: %w", pullRequest.Number, err)
			}

			reviews, err := s.syncReviews(ctx, jobID, owner, repoName, pullRequest.Number, cutoff)
			if err != nil {
				return 0, err
			}

			if err := s.store.UpsertPullRequestBundle(ctx, buildPullRequestInput(detail, pullRequest, repositoryID), buildReviewInputs(reviews)); err != nil {
				return 0, fmt.Errorf("persist pull request %d: %w", pullRequest.Number, err)
			}
			files, err := s.syncPullRequestFiles(ctx, jobID, owner, repoName, pullRequest.Number)
			if err != nil {
				return 0, err
			}
			if err := s.store.ReplacePullRequestFiles(ctx, repositoryID, fallbackInt64(detail.ID, pullRequest.ID), buildFileChangeInputs(files)); err != nil {
				return 0, fmt.Errorf("persist pull request files %d: %w", pullRequest.Number, err)
			}

			total++
		}

		if result.NextPage == 0 {
			if err := s.completeCheckpoint(ctx, jobID, repositoryID, "pull_requests", "page"); err != nil {
				return 0, err
			}
			return total, nil
		}
		if err := s.storeProgressCheckpoint(ctx, jobID, repositoryID, "pull_requests", "page", result.NextPage); err != nil {
			return 0, err
		}
		page = result.NextPage
	}
}

func (s *Service) syncPullRequestFiles(ctx context.Context, jobID string, owner string, repoName string, pullNumber int) ([]githubclient.PullRequestFile, error) {
	items := make([]githubclient.PullRequestFile, 0)
	key := fmt.Sprintf("pull_%d", pullNumber)
	page, err := s.resumePage(ctx, jobID, "changed_files", key)
	if err != nil {
		return nil, err
	}

	for {
		if err := s.ensureNotCanceled(ctx, jobID); err != nil {
			return nil, err
		}
		result, err := s.githubClient.ListPullRequestFiles(ctx, owner, repoName, pullNumber, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, fmt.Errorf("list pull request files for pull request %d: %w", pullNumber, err)
		}
		if err := s.maybeThrottleGitHub(ctx, result.RateLimit); err != nil {
			return nil, err
		}

		items = append(items, result.Items...)

		if result.NextPage == 0 {
			if err := s.completeCheckpoint(ctx, jobID, "", "changed_files", key); err != nil {
				return nil, err
			}
			return items, nil
		}
		if err := s.storeProgressCheckpoint(ctx, jobID, "", "changed_files", key, result.NextPage); err != nil {
			return nil, err
		}
		page = result.NextPage
	}
}

func (s *Service) syncReviews(ctx context.Context, jobID string, owner string, repoName string, pullNumber int, cutoff *time.Time) ([]githubclient.Review, error) {
	items := make([]githubclient.Review, 0)
	key := fmt.Sprintf("pull_%d", pullNumber)
	page, err := s.resumePage(ctx, jobID, "reviews", key)
	if err != nil {
		return nil, err
	}

	for {
		if err := s.ensureNotCanceled(ctx, jobID); err != nil {
			return nil, err
		}
		result, err := s.githubClient.ListReviews(ctx, owner, repoName, pullNumber, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, fmt.Errorf("list reviews for pull request %d: %w", pullNumber, err)
		}
		if err := s.maybeThrottleGitHub(ctx, result.RateLimit); err != nil {
			return nil, err
		}

		for _, review := range result.Items {
			if !includeReview(review, cutoff) {
				continue
			}
			items = append(items, review)
		}

		if result.NextPage == 0 {
			if err := s.completeCheckpoint(ctx, jobID, "", "reviews", key); err != nil {
				return nil, err
			}
			return items, nil
		}
		if err := s.storeProgressCheckpoint(ctx, jobID, "", "reviews", key, result.NextPage); err != nil {
			return nil, err
		}
		page = result.NextPage
	}
}

func (s *Service) syncCommits(ctx context.Context, jobID string, repositoryID string, owner string, repoName string, cutoff *time.Time) (int, error) {
	total := 0
	page, err := s.resumePage(ctx, jobID, "commits", "page")
	if err != nil {
		return 0, err
	}

	for {
		if err := s.ensureNotCanceled(ctx, jobID); err != nil {
			return 0, err
		}
		result, err := s.githubClient.ListCommits(ctx, owner, repoName, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return 0, fmt.Errorf("list commits: %w", err)
		}
		if err := s.maybeThrottleGitHub(ctx, result.RateLimit); err != nil {
			return 0, err
		}

		payload := make([]commitEventInput, 0, len(result.Items))
		for _, commit := range result.Items {
			if !includeCommit(commit, cutoff) {
				continue
			}
			payload = append(payload, buildCommitEventInput(commit))
			total++
		}
		if err := s.store.UpsertCommitEvents(ctx, repositoryID, payload); err != nil {
			return 0, fmt.Errorf("persist commit batch: %w", err)
		}

		if result.NextPage == 0 {
			if err := s.completeCheckpoint(ctx, jobID, repositoryID, "commits", "page"); err != nil {
				return 0, err
			}
			return total, nil
		}
		if err := s.storeProgressCheckpoint(ctx, jobID, repositoryID, "commits", "page", result.NextPage); err != nil {
			return 0, err
		}
		page = result.NextPage
	}
}

func (s *Service) syncWorkflowRuns(ctx context.Context, jobID string, repositoryID string, owner string, repoName string, cutoff *time.Time) (int, error) {
	total := 0
	page, err := s.resumePage(ctx, jobID, "workflows", "page")
	if err != nil {
		return 0, err
	}

	for {
		if err := s.ensureNotCanceled(ctx, jobID); err != nil {
			return 0, err
		}
		result, err := s.githubClient.ListWorkflowRuns(ctx, owner, repoName, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return 0, fmt.Errorf("list workflow runs: %w", err)
		}
		if err := s.maybeThrottleGitHub(ctx, result.RateLimit); err != nil {
			return 0, err
		}

		payload := make([]workflowRunInput, 0, len(result.Items))
		for _, run := range result.Items {
			if !includeWorkflowRun(run, cutoff) {
				continue
			}
			payload = append(payload, buildWorkflowRunInput(run))
			total++
		}
		if err := s.store.UpsertWorkflowRuns(ctx, repositoryID, payload); err != nil {
			return 0, fmt.Errorf("persist workflow run batch: %w", err)
		}

		if result.NextPage == 0 {
			if err := s.completeCheckpoint(ctx, jobID, repositoryID, "workflows", "page"); err != nil {
				return 0, err
			}
			return total, nil
		}
		if err := s.storeProgressCheckpoint(ctx, jobID, repositoryID, "workflows", "page", result.NextPage); err != nil {
			return 0, err
		}
		page = result.NextPage
	}
}

func (s *Service) syncDeployments(ctx context.Context, jobID string, repositoryID string, owner string, repoName string, cutoff *time.Time) (int, error) {
	total := 0
	page, err := s.resumePage(ctx, jobID, "deployments", "page")
	if err != nil {
		return 0, err
	}

	for {
		if err := s.ensureNotCanceled(ctx, jobID); err != nil {
			return 0, err
		}
		result, err := s.githubClient.ListDeployments(ctx, owner, repoName, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return 0, fmt.Errorf("list deployments: %w", err)
		}
		if err := s.maybeThrottleGitHub(ctx, result.RateLimit); err != nil {
			return 0, err
		}

		payload := make([]deploymentInput, 0, len(result.Items))
		for _, deployment := range result.Items {
			if !includeDeployment(deployment, cutoff) {
				continue
			}
			statuses, err := s.githubClient.ListDeploymentStatuses(ctx, owner, repoName, deployment.ID, githubclient.ListOptions{
				Page:    1,
				PerPage: 100,
			})
			if err != nil {
				return 0, fmt.Errorf("list deployment statuses for deployment %d: %w", deployment.ID, err)
			}
			if err := s.maybeThrottleGitHub(ctx, statuses.RateLimit); err != nil {
				return 0, err
			}
			latestStatus := latestDeploymentStatus(statuses.Items)
			if latestStatus == nil {
				continue
			}
			payload = append(payload, buildDeploymentInput(deployment, *latestStatus))
			total++
		}
		if err := s.store.UpsertDeployments(ctx, repositoryID, payload); err != nil {
			return 0, fmt.Errorf("persist deployment batch: %w", err)
		}

		if result.NextPage == 0 {
			if err := s.completeCheckpoint(ctx, jobID, repositoryID, "deployments", "page"); err != nil {
				return 0, err
			}
			return total, nil
		}
		if err := s.storeProgressCheckpoint(ctx, jobID, repositoryID, "deployments", "page", result.NextPage); err != nil {
			return 0, err
		}
		page = result.NextPage
	}
}

func (s *Service) maybeThrottleGitHub(ctx context.Context, rate githubclient.RateLimit) error {
	if s == nil || s.sleep == nil || s.minRateLimitRemaining <= 0 {
		return nil
	}
	if rate.Remaining > s.minRateLimitRemaining {
		return nil
	}
	waitFor := 2 * time.Second
	if !rate.ResetAt.IsZero() {
		if untilReset := time.Until(rate.ResetAt); untilReset > 0 {
			waitFor = untilReset
		}
	}
	return s.sleep(ctx, waitFor)
}

func sleepWithContext(ctx context.Context, waitFor time.Duration) error {
	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) resumePage(ctx context.Context, jobID string, resourceType string, key string) (int, error) {
	checkpoint, err := s.store.GetCheckpoint(ctx, jobID, resourceType, key)
	if err != nil {
		return 0, err
	}
	if checkpoint == nil || checkpoint.Value == nil || strings.TrimSpace(*checkpoint.Value) == "" {
		return 1, nil
	}

	page, parseErr := strconv.Atoi(strings.TrimSpace(*checkpoint.Value))
	if parseErr != nil || page < 1 {
		return 1, nil
	}
	return page, nil
}

func (s *Service) storeProgressCheckpoint(ctx context.Context, jobID string, repositoryID string, resourceType string, key string, nextPage int) error {
	value := strconv.Itoa(nextPage)
	return s.store.UpsertCheckpoint(ctx, jobID, repositoryID, resourceType, key, &value, "running", timePtr(s.now().UTC()))
}

func (s *Service) completeCheckpoint(ctx context.Context, jobID string, repositoryID string, resourceType string, key string) error {
	return s.store.UpsertCheckpoint(ctx, jobID, repositoryID, resourceType, key, nil, "completed", timePtr(s.now().UTC()))
}

func reqIdempotencyKey(options syncOptions) string {
	return strings.TrimSpace(options.idempotencyKey)
}

func timePtr(value time.Time) *time.Time {
	utc := value.UTC()
	return &utc
}

func (s *Service) persistSyncOptions(ctx context.Context, jobID string, repositoryID string, options syncOptions) error {
	mode := options.mode
	if strings.TrimSpace(mode) == "" {
		mode = ModeIncremental
	}
	if err := s.store.UpsertCheckpoint(ctx, jobID, repositoryID, "job", "mode", &mode, "pending", timePtr(s.now().UTC())); err != nil {
		return err
	}
	if options.from != nil {
		formatted := options.from.UTC().Format("2006-01-02")
		if err := s.store.UpsertCheckpoint(ctx, jobID, repositoryID, "job", "from", &formatted, "pending", timePtr(s.now().UTC())); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) loadSyncOptions(ctx context.Context, jobID string) (syncOptions, error) {
	options := syncOptions{mode: ModeIncremental}

	modeCheckpoint, err := s.store.GetCheckpoint(ctx, jobID, "job", "mode")
	if err != nil {
		return syncOptions{}, err
	}
	if modeCheckpoint != nil && modeCheckpoint.Value != nil && strings.TrimSpace(*modeCheckpoint.Value) != "" {
		options.mode = strings.TrimSpace(*modeCheckpoint.Value)
	}

	fromCheckpoint, err := s.store.GetCheckpoint(ctx, jobID, "job", "from")
	if err != nil {
		return syncOptions{}, err
	}
	if fromCheckpoint != nil && fromCheckpoint.Value != nil && strings.TrimSpace(*fromCheckpoint.Value) != "" {
		parsed, parseErr := time.Parse("2006-01-02", strings.TrimSpace(*fromCheckpoint.Value))
		if parseErr == nil {
			utc := parsed.UTC()
			options.from = &utc
		}
	}

	return options, nil
}

func (s *Service) ensureNotCanceled(ctx context.Context, jobID string) error {
	job, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status == StatusCanceled {
		return errSyncCanceled
	}
	return nil
}

func (s *Service) canceledJobResult(ctx context.Context, jobID string, err error) (SyncJobResponse, error) {
	if errors.Is(err, errSyncCanceled) {
		return s.store.GetByID(ctx, jobID)
	}
	return SyncJobResponse{}, err
}

func splitFullName(fullName string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(fullName), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("repository full name must be in owner/repository format")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func resolveSyncCutoff(target repositoryTarget, options syncOptions) *time.Time {
	if options.mode == ModeFull {
		return nil
	}
	if options.from != nil {
		return options.from
	}
	return target.LastSyncedAt
}

func includePullRequest(item githubclient.PullRequest, cutoff *time.Time) bool {
	if cutoff == nil {
		return true
	}
	return !item.UpdatedAt.Before(*cutoff)
}

func includeReview(item githubclient.Review, cutoff *time.Time) bool {
	if cutoff == nil || item.SubmittedAt == nil {
		return true
	}
	return !item.SubmittedAt.Before(*cutoff)
}

func includeCommit(item githubclient.Commit, cutoff *time.Time) bool {
	if cutoff == nil {
		return true
	}
	return !item.Commit.Author.Date.Before(*cutoff)
}

func includeWorkflowRun(item githubclient.WorkflowRun, cutoff *time.Time) bool {
	if cutoff == nil {
		return true
	}
	timestamp := item.UpdatedAt
	if timestamp.IsZero() {
		timestamp = item.CreatedAt
	}
	return !timestamp.Before(*cutoff)
}

func includeDeployment(item githubclient.Deployment, cutoff *time.Time) bool {
	if cutoff == nil {
		return true
	}
	timestamp := item.UpdatedAt
	if timestamp.IsZero() {
		timestamp = item.CreatedAt
	}
	return !timestamp.Before(*cutoff)
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func archivedAt(archived bool, at time.Time) *time.Time {
	if !archived {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func buildPullRequestInput(detail githubclient.PullRequest, fallback githubclient.PullRequest, repositoryID string) pullRequestInput {
	author := detail.User.Login
	if strings.TrimSpace(author) == "" {
		author = fallback.User.Login
	}

	title := detail.Title
	if strings.TrimSpace(title) == "" {
		title = fallback.Title
	}

	state := detail.State
	if strings.TrimSpace(state) == "" {
		state = fallback.State
	}

	createdAt := detail.CreatedAt
	if createdAt.IsZero() {
		createdAt = fallback.CreatedAt
	}

	closedAt := detail.ClosedAt
	if closedAt == nil {
		closedAt = fallback.ClosedAt
	}

	mergedAt := detail.MergedAt
	if mergedAt == nil {
		mergedAt = fallback.MergedAt
	}

	return pullRequestInput{
		RepositoryID: repositoryID,
		GitHubPRID:   fallbackInt64(detail.ID, fallback.ID),
		Number:       fallbackInt(detail.Number, fallback.Number),
		Title:        title,
		Author:       author,
		State:        state,
		IsDraft:      detail.Draft || fallback.Draft,
		CreatedAt:    createdAt.UTC(),
		MergedAt:     normalizeTime(mergedAt),
		ClosedAt:     normalizeTime(closedAt),
		Additions:    detail.Additions,
		Deletions:    detail.Deletions,
		FilesChanged: detail.ChangedFiles,
	}
}

func buildReviewInputs(reviews []githubclient.Review) []pullRequestReviewInput {
	if len(reviews) == 0 {
		return nil
	}

	firstReviewAt := earliestSubmittedAt(reviews)
	items := make([]pullRequestReviewInput, 0, len(reviews))
	for _, review := range reviews {
		items = append(items, pullRequestReviewInput{
			GitHubReviewID:    review.ID,
			Reviewer:          review.User.Login,
			ReviewRequestedAt: nil,
			FirstReviewAt:     firstReviewAt,
			ReviewSubmittedAt: normalizeTime(review.SubmittedAt),
			State:             review.State,
		})
	}
	return items
}

func buildFileChangeInputs(files []githubclient.PullRequestFile) []fileChangeInput {
	if len(files) == 0 {
		return nil
	}
	items := make([]fileChangeInput, 0, len(files))
	for _, file := range files {
		items = append(items, fileChangeInput{
			FilePath:    file.Filename,
			Additions:   file.Additions,
			Deletions:   file.Deletions,
			CommitCount: 1,
		})
	}
	return items
}

func buildCommitEventInput(commit githubclient.Commit) commitEventInput {
	author := commit.Commit.Author.Name
	if strings.TrimSpace(author) == "" && commit.Author != nil {
		author = commit.Author.Login
	}
	return commitEventInput{
		GitHubCommitSHA: strings.TrimSpace(commit.SHA),
		Author:          strings.TrimSpace(author),
		AuthorEmail:     strings.TrimSpace(commit.Commit.Author.Email),
		Message:         strings.TrimSpace(commit.Commit.Message),
		AuthoredAt:      commit.Commit.Author.Date.UTC(),
	}
}

func buildWorkflowRunInput(run githubclient.WorkflowRun) workflowRunInput {
	return workflowRunInput{
		GitHubWorkflowRunID: run.ID,
		WorkflowName:        run.Name,
		Status:              run.Status,
		Conclusion:          run.Conclusion,
		StartedAt:           normalizeTime(run.RunStartedAt),
		CompletedAt:         workflowCompletedAt(run),
	}
}

func buildDeploymentInput(deployment githubclient.Deployment, status githubclient.DeploymentStatus) deploymentInput {
	deployedAt := status.UpdatedAt
	if deployedAt.IsZero() {
		deployedAt = status.CreatedAt
	}
	if deployedAt.IsZero() {
		deployedAt = deployment.UpdatedAt
	}
	if deployedAt.IsZero() {
		deployedAt = deployment.CreatedAt
	}
	return deploymentInput{
		GitHubDeploymentID: deployment.ID,
		Environment:        deployment.Environment,
		Status:             status.State,
		DeployedAt:         deployedAt.UTC(),
	}
}

func workflowCompletedAt(run githubclient.WorkflowRun) *time.Time {
	if run.Conclusion == "" {
		return nil
	}
	if run.UpdatedAt.IsZero() {
		return nil
	}
	value := run.UpdatedAt.UTC()
	return &value
}

func latestDeploymentStatus(items []githubclient.DeploymentStatus) *githubclient.DeploymentStatus {
	var latest *githubclient.DeploymentStatus
	for i := range items {
		candidate := items[i]
		if latest == nil || deploymentStatusTime(candidate).After(deploymentStatusTime(*latest)) {
			latest = &candidate
		}
	}
	return latest
}

func deploymentStatusTime(item githubclient.DeploymentStatus) time.Time {
	if !item.UpdatedAt.IsZero() {
		return item.UpdatedAt.UTC()
	}
	return item.CreatedAt.UTC()
}

func earliestSubmittedAt(reviews []githubclient.Review) *time.Time {
	var earliest *time.Time
	for _, review := range reviews {
		if review.SubmittedAt == nil {
			continue
		}
		candidate := review.SubmittedAt.UTC()
		if earliest == nil || candidate.Before(*earliest) {
			value := candidate
			earliest = &value
		}
	}
	return earliest
}

func normalizeTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func fallbackInt(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func fallbackInt64(value int64, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
}
