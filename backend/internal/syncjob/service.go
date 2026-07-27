package syncjob

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/githubclient"
)

type store interface {
	EnsureRepositoryExists(context.Context, string) error
	HasActiveJob(context.Context, string) (bool, error)
	Create(context.Context, createParams) (SyncJobResponse, error)
	GetByID(context.Context, string) (SyncJobResponse, error)
	ListByRepository(context.Context, ListParams) (ListResult, error)
	GetRepositoryTarget(context.Context, string) (repositoryTarget, error)
	MarkRunning(context.Context, string, int, time.Time) (SyncJobResponse, error)
	UpdateProgress(context.Context, string, int, time.Time) (SyncJobResponse, error)
	MarkFailed(context.Context, string, string, time.Time) (SyncJobResponse, error)
	SyncRepositoryMetadata(context.Context, string, repositoryMetadata, time.Time) error
	UpsertPullRequestBundle(context.Context, pullRequestInput, []pullRequestReviewInput) error
	Complete(context.Context, string, string, time.Time) (SyncJobResponse, error)
}

type Service struct {
	store        store
	githubClient githubclient.Client
	publisher    completionPublisher
	now          func() time.Time
}

type SyncCompletedEvent struct {
	RepositoryID string    `json:"repositoryId"`
	SyncJobID    string    `json:"syncJobId"`
	OccurredAt   time.Time `json:"occurredAt"`
	EventType    string    `json:"eventType"`
}

type completionPublisher interface {
	PublishRepositorySyncCompleted(context.Context, SyncCompletedEvent) error
}

func NewService(store store, githubClient githubclient.Client) *Service {
	return &Service{
		store:        store,
		githubClient: githubClient,
		now:          time.Now,
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

	job, err := s.store.Create(ctx, createParams{RepositoryID: repositoryID})
	if err != nil {
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
	return s.run(ctx, job, syncOptions{mode: ModeIncremental})
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
	mode string
	from *time.Time
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

	return syncOptions{mode: mode, from: from}, nil
}

func validateListParams(params ListParams) error {
	var issues []ValidationIssue

	if params.Status != "" &&
		params.Status != StatusPending &&
		params.Status != StatusRunning &&
		params.Status != StatusCompleted &&
		params.Status != StatusFailed {
		issues = append(issues, ValidationIssue{Field: "status", Message: "must be one of pending, running, completed, failed"})
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
	startedAt := s.now().UTC()
	job, err := s.store.MarkRunning(ctx, job.ID, 5, startedAt)
	if err != nil {
		return SyncJobResponse{}, err
	}

	target, err := s.store.GetRepositoryTarget(ctx, job.RepositoryID)
	if err != nil {
		return s.failJob(ctx, job, err)
	}

	owner, repoName, err := splitFullName(target.FullName)
	if err != nil {
		return s.failJob(ctx, job, err)
	}

	cutoff := resolveSyncCutoff(target, options)

	repo, err := s.githubClient.GetRepository(ctx, owner, repoName)
	if err != nil {
		return s.failJob(ctx, job, fmt.Errorf("sync repository metadata: %w", err))
	}

	if err := s.store.SyncRepositoryMetadata(ctx, job.RepositoryID, repositoryMetadata{
		Name:          repo.Name,
		FullName:      repo.FullName,
		DefaultBranch: stringPtr(repo.DefaultBranch),
		IsActive:      !repo.Archived,
		ArchivedAt:    archivedAt(repo.Archived, startedAt),
	}, startedAt); err != nil {
		return s.failJob(ctx, job, err)
	}

	job, err = s.store.UpdateProgress(ctx, job.ID, 25, s.now().UTC())
	if err != nil {
		return SyncJobResponse{}, err
	}

	pullRequests, err := s.syncPullRequests(ctx, job.RepositoryID, owner, repoName, cutoff)
	if err != nil {
		return s.failJob(ctx, job, err)
	}

	if pullRequests > 0 {
		job, err = s.store.UpdateProgress(ctx, job.ID, 75, s.now().UTC())
	} else {
		job, err = s.store.UpdateProgress(ctx, job.ID, 60, s.now().UTC())
	}
	if err != nil {
		return SyncJobResponse{}, err
	}

	if _, err := s.syncCommits(ctx, owner, repoName, cutoff); err != nil {
		return s.failJob(ctx, job, err)
	}

	completedAt := s.now().UTC()
	completedJob, err := s.store.Complete(ctx, job.ID, job.RepositoryID, completedAt)
	if err != nil {
		return SyncJobResponse{}, err
	}

	if s.publisher != nil {
		if err := s.publisher.PublishRepositorySyncCompleted(ctx, SyncCompletedEvent{
			EventType:    "repository.sync.completed",
			RepositoryID: job.RepositoryID,
			SyncJobID:    job.ID,
			OccurredAt:   completedAt,
		}); err != nil {
			return completedJob, fmt.Errorf("trigger metrics calculation: %w", err)
		}
	}

	return completedJob, nil
}

func (s *Service) failJob(ctx context.Context, job SyncJobResponse, err error) (SyncJobResponse, error) {
	failedAt := s.now().UTC()
	failedJob, markErr := s.store.MarkFailed(ctx, job.ID, err.Error(), failedAt)
	if markErr != nil {
		return SyncJobResponse{}, markErr
	}
	return failedJob, nil
}

func (s *Service) syncPullRequests(ctx context.Context, repositoryID string, owner string, repoName string, cutoff *time.Time) (int, error) {
	total := 0
	page := 1

	for {
		result, err := s.githubClient.ListPullRequests(ctx, owner, repoName, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
			State:   "all",
		})
		if err != nil {
			return 0, fmt.Errorf("list pull requests: %w", err)
		}

		for _, pullRequest := range result.Items {
			if !includePullRequest(pullRequest, cutoff) {
				continue
			}

			detail, err := s.githubClient.GetPullRequest(ctx, owner, repoName, pullRequest.Number)
			if err != nil {
				return 0, fmt.Errorf("get pull request %d: %w", pullRequest.Number, err)
			}

			reviews, err := s.syncReviews(ctx, owner, repoName, pullRequest.Number, cutoff)
			if err != nil {
				return 0, err
			}

			if err := s.store.UpsertPullRequestBundle(ctx, buildPullRequestInput(detail, pullRequest, repositoryID), buildReviewInputs(reviews)); err != nil {
				return 0, fmt.Errorf("persist pull request %d: %w", pullRequest.Number, err)
			}

			total++
		}

		if result.NextPage == 0 {
			return total, nil
		}
		page = result.NextPage
	}
}

func (s *Service) syncReviews(ctx context.Context, owner string, repoName string, pullNumber int, cutoff *time.Time) ([]githubclient.Review, error) {
	items := make([]githubclient.Review, 0)
	page := 1

	for {
		result, err := s.githubClient.ListReviews(ctx, owner, repoName, pullNumber, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, fmt.Errorf("list reviews for pull request %d: %w", pullNumber, err)
		}

		for _, review := range result.Items {
			if !includeReview(review, cutoff) {
				continue
			}
			items = append(items, review)
		}

		if result.NextPage == 0 {
			return items, nil
		}
		page = result.NextPage
	}
}

func (s *Service) syncCommits(ctx context.Context, owner string, repoName string, cutoff *time.Time) (int, error) {
	total := 0
	page := 1

	for {
		result, err := s.githubClient.ListCommits(ctx, owner, repoName, githubclient.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return 0, fmt.Errorf("list commits: %w", err)
		}

		for _, commit := range result.Items {
			if !includeCommit(commit, cutoff) {
				continue
			}
			total++
		}

		if result.NextPage == 0 {
			return total, nil
		}
		page = result.NextPage
	}
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
