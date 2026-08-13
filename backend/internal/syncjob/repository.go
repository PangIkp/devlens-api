package syncjob

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db      *postgres.DB
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{
		db:      db,
		queries: db.Queries(),
	}
}

func (r *Repository) EnsureRepositoryExists(ctx context.Context, repositoryID string) error {
	exists, err := r.queries.SyncJobRepositoryExists(ctx, parseUUID(repositoryID))
	if err != nil {
		return fmt.Errorf("check repository exists: %w", err)
	}
	if !exists {
		return ErrRepositoryNotFound
	}
	return nil
}

func (r *Repository) HasActiveJob(ctx context.Context, repositoryID string) (bool, error) {
	active, err := r.queries.HasActiveSyncJob(ctx, parseUUID(repositoryID))
	if err != nil {
		return false, fmt.Errorf("check active sync jobs: %w", err)
	}
	return active, nil
}

func (r *Repository) Create(ctx context.Context, params createParams) (SyncJobResponse, error) {
	if params.IdempotencyKey != nil && strings.TrimSpace(*params.IdempotencyKey) != "" {
		job, err := r.getByIdempotencyKey(ctx, strings.TrimSpace(*params.IdempotencyKey))
		if err != nil {
			return SyncJobResponse{}, err
		}
		if job != nil {
			return *job, nil
		}
	}

	row := r.db.Pool().QueryRow(ctx, `
		INSERT INTO sync_jobs (
			id, repository_id, job_type, status, progress, idempotency_key, triggered_by, error_message, started_at, finished_at, created_at, updated_at
		) VALUES (
			$1, $2, 'repository_sync', $3, $4, $5, $6, $7, $8, $9, NOW(), NULL
		)
		RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at`,
		newUUID(),
		parseUUID(params.RepositoryID),
		StatusPending,
		0,
		textPointerValue(params.IdempotencyKey),
		nullableUUID(params.TriggeredBy),
		pgtype.Text{},
		pgtype.Timestamptz{},
		pgtype.Timestamptz{},
	)

	var created sqlcgen.CreateSyncJobRow
	if err := row.Scan(
		&created.ID,
		&created.RepositoryID,
		&created.Status,
		&created.Progress,
		&created.TriggeredBy,
		&created.ErrorMessage,
		&created.StartedAt,
		&created.FinishedAt,
		&created.CreatedAt,
		&created.UpdatedAt,
	); err != nil {
		if params.IdempotencyKey != nil && strings.TrimSpace(*params.IdempotencyKey) != "" {
			job, lookupErr := r.getByIdempotencyKey(ctx, strings.TrimSpace(*params.IdempotencyKey))
			if lookupErr == nil && job != nil {
				return *job, nil
			}
		}
		return SyncJobResponse{}, fmt.Errorf("create sync job: %w", err)
	}
	if _, err := r.db.Pool().Exec(ctx, `
		UPDATE repositories
		SET initial_sync_status = 'syncing',
		    sync_error_message = NULL,
		    initial_sync_completed_at = NULL,
		    updated_at = NOW()
		WHERE id = $1`, parseUUID(params.RepositoryID)); err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark repository syncing: %w", err)
	}
	return syncJobResponseFromCreateRow(created), nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (SyncJobResponse, error) {
	row, err := r.queries.GetSyncJobByID(ctx, parseUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SyncJobResponse{}, ErrSyncJobNotFound
		}
		return SyncJobResponse{}, fmt.Errorf("get sync job: %w", err)
	}
	return syncJobResponseFromGetRow(row), nil
}

func (r *Repository) ListPendingIDs(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.queries.ListPendingSyncJobs(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list pending sync jobs: %w", err)
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.String())
	}
	return ids, nil
}

func (r *Repository) ListByRepository(ctx context.Context, params ListParams) (ListResult, error) {
	rows, err := r.queries.ListSyncJobsByRepository(ctx, sqlcgen.ListSyncJobsByRepositoryParams{
		RepositoryID: parseUUID(params.RepositoryID),
		Limit:        int32(params.PageSize),
		Offset:       int32((params.Page - 1) * params.PageSize),
		Status:       pgtype.Text{String: strings.TrimSpace(params.Status), Valid: strings.TrimSpace(params.Status) != ""},
		SortOrder:    params.SortOrder,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list sync jobs: %w", err)
	}

	totalItems, err := r.queries.CountSyncJobsByRepository(ctx, sqlcgen.CountSyncJobsByRepositoryParams{
		RepositoryID: parseUUID(params.RepositoryID),
		Status:       pgtype.Text{String: strings.TrimSpace(params.Status), Valid: strings.TrimSpace(params.Status) != ""},
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("count sync jobs: %w", err)
	}

	result := ListResult{
		Items:      make([]SyncJobResponse, 0, len(rows)),
		TotalItems: int(totalItems),
	}
	for _, row := range rows {
		result.Items = append(result.Items, syncJobResponseFromListRow(row))
	}

	return result, nil
}

func (r *Repository) Retry(ctx context.Context, id string, at time.Time) (SyncJobResponse, error) {
	row, err := r.db.Pool().Query(ctx, `
		UPDATE sync_jobs
		SET status = 'pending',
		    progress = 0,
		    error_message = NULL,
		    started_at = NULL,
		    finished_at = NULL,
		    updated_at = $2
		WHERE id = $1
		RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at`,
		parseUUID(id),
		toNullableTimestamp(&at),
	)
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("retry sync job: %w", err)
	}
	defer row.Close()

	if !row.Next() {
		return SyncJobResponse{}, ErrSyncJobNotFound
	}

	var job SyncJobResponse
	var jobID, repositoryID, triggeredBy pgtype.UUID
	var errorMessage pgtype.Text
	var startedAt, finishedAt, createdAt, updatedAt pgtype.Timestamptz
	var status string
	var progress int32
	if err := row.Scan(&jobID, &repositoryID, &status, &progress, &triggeredBy, &errorMessage, &startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		return SyncJobResponse{}, fmt.Errorf("scan retried sync job: %w", err)
	}
	job = buildSyncJobResponse(jobID, repositoryID, status, progress, triggeredBy, errorMessage, startedAt, finishedAt, createdAt, updatedAt)

	if _, err := r.db.Pool().Exec(ctx, `
		UPDATE repositories
		SET initial_sync_status = 'syncing',
		    sync_error_message = NULL,
		    updated_at = NOW()
		WHERE id = $1`, parseUUID(job.RepositoryID)); err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark repository syncing after retry: %w", err)
	}

	return job, nil
}

func (r *Repository) Cancel(ctx context.Context, id string, at time.Time) (SyncJobResponse, error) {
	job, err := r.GetByID(ctx, id)
	if err != nil {
		return SyncJobResponse{}, err
	}

	row := r.db.Pool().QueryRow(ctx, `
		UPDATE sync_jobs
		SET status = 'canceled',
		    error_message = NULL,
		    finished_at = $2,
		    updated_at = $2
		WHERE id = $1
		RETURNING id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at`,
		parseUUID(id),
		toNullableTimestamp(&at),
	)

	var canceled sqlcgen.GetSyncJobByIDRow
	if err := row.Scan(
		&canceled.ID,
		&canceled.RepositoryID,
		&canceled.Status,
		&canceled.Progress,
		&canceled.TriggeredBy,
		&canceled.ErrorMessage,
		&canceled.StartedAt,
		&canceled.FinishedAt,
		&canceled.CreatedAt,
		&canceled.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SyncJobResponse{}, ErrSyncJobNotFound
		}
		return SyncJobResponse{}, fmt.Errorf("cancel sync job: %w", err)
	}

	if _, err := r.db.Pool().Exec(ctx, `
		UPDATE repositories
		SET initial_sync_status = 'selected',
		    sync_error_message = NULL,
		    updated_at = NOW()
		WHERE id = $1`, parseUUID(job.RepositoryID)); err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark repository canceled sync state: %w", err)
	}

	return syncJobResponseFromGetRow(canceled), nil
}

func (r *Repository) GetRepositoryTarget(ctx context.Context, repositoryID string) (repositoryTarget, error) {
	row, err := r.queries.GetSyncJobRepositoryTarget(ctx, parseUUID(repositoryID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repositoryTarget{}, ErrRepositoryNotFound
		}
		return repositoryTarget{}, fmt.Errorf("get repository sync target: %w", err)
	}

	return repositoryTarget{
		ID:           row.ID.String(),
		FullName:     row.FullName,
		LastSyncedAt: optionalTimeValue(row.LastSyncedAt),
	}, nil
}

func (r *Repository) GetRepositoryOrganizationID(ctx context.Context, repositoryID string) (string, error) {
	var organizationID string
	err := r.db.Pool().QueryRow(ctx, `SELECT organization_id::text FROM repositories WHERE id = $1`, parseUUID(repositoryID)).Scan(&organizationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrRepositoryNotFound
		}
		return "", fmt.Errorf("get repository organization id: %w", err)
	}
	return organizationID, nil
}

func (r *Repository) MarkRunning(ctx context.Context, id string, progress int, at time.Time) (SyncJobResponse, error) {
	row, err := r.queries.UpdateSyncJobRunning(ctx, sqlcgen.UpdateSyncJobRunningParams{
		ID:        parseUUID(id),
		Progress:  int32(progress),
		UpdatedAt: toNullableTimestamp(&at),
	})
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark sync job running: %w", err)
	}
	return syncJobResponseFromRunningRow(row), nil
}

func (r *Repository) UpdateProgress(ctx context.Context, id string, progress int, at time.Time) (SyncJobResponse, error) {
	row, err := r.queries.UpdateSyncJobProgress(ctx, sqlcgen.UpdateSyncJobProgressParams{
		ID:        parseUUID(id),
		Progress:  int32(progress),
		UpdatedAt: toNullableTimestamp(&at),
	})
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("update sync job progress: %w", err)
	}
	return syncJobResponseFromProgressRow(row), nil
}

func (r *Repository) MarkFailed(ctx context.Context, id string, message string, at time.Time) (SyncJobResponse, error) {
	job, err := r.GetByID(ctx, id)
	if err != nil {
		return SyncJobResponse{}, err
	}

	row, err := r.queries.UpdateSyncJobFailed(ctx, sqlcgen.UpdateSyncJobFailedParams{
		ID:           parseUUID(id),
		ErrorMessage: pgtype.Text{String: message, Valid: true},
		FinishedAt:   toNullableTimestamp(&at),
	})
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark sync job failed: %w", err)
	}
	if _, err := r.db.Pool().Exec(ctx, `
		UPDATE repositories
		SET initial_sync_status = 'sync_failed',
		    sync_error_message = $2,
		    updated_at = NOW()
		WHERE id = $1`, parseUUID(job.RepositoryID), message); err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark repository sync failed: %w", err)
	}
	return syncJobResponseFromFailedRow(row), nil
}

func (r *Repository) SyncRepositoryMetadata(ctx context.Context, repositoryID string, metadata repositoryMetadata, at time.Time) error {
	err := r.queries.SyncRepositoryMetadata(ctx, sqlcgen.SyncRepositoryMetadataParams{
		ID:            parseUUID(repositoryID),
		Name:          metadata.Name,
		FullName:      metadata.FullName,
		DefaultBranch: textPointerValue(metadata.DefaultBranch),
		IsActive:      metadata.IsActive,
		ArchivedAt:    toNullableTimestamp(metadata.ArchivedAt),
		UpdatedAt:     toNullableTimestamp(&at),
	})
	if err != nil {
		return fmt.Errorf("sync repository metadata: %w", err)
	}
	return nil
}

func (r *Repository) UpsertPullRequestBundle(ctx context.Context, pullRequest pullRequestInput, reviews []pullRequestReviewInput) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin pull request upsert transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := r.queries.WithTx(tx)
	pullRequestID, err := queries.UpsertPullRequest(ctx, sqlcgen.UpsertPullRequestParams{
		ID:           newUUID(),
		RepositoryID: parseUUID(pullRequest.RepositoryID),
		GithubPrID:   pullRequest.GitHubPRID,
		Number:       int32(pullRequest.Number),
		Title:        pullRequest.Title,
		Author:       pullRequest.Author,
		State:        pullRequest.State,
		IsDraft:      pullRequest.IsDraft,
		CreatedAt:    toNullableTimestamp(&pullRequest.CreatedAt),
		MergedAt:     toNullableTimestamp(pullRequest.MergedAt),
		ClosedAt:     toNullableTimestamp(pullRequest.ClosedAt),
		Additions:    int32(pullRequest.Additions),
		Deletions:    int32(pullRequest.Deletions),
		FilesChanged: int32(pullRequest.FilesChanged),
	})
	if err != nil {
		return fmt.Errorf("upsert pull request: %w", err)
	}

	for _, review := range reviews {
		if err := queries.UpsertPullRequestReview(ctx, sqlcgen.UpsertPullRequestReviewParams{
			ID:                newUUID(),
			PullRequestID:     pullRequestID,
			GithubReviewID:    review.GitHubReviewID,
			Reviewer:          review.Reviewer,
			ReviewRequestedAt: toNullableTimestamp(review.ReviewRequestedAt),
			FirstReviewAt:     toNullableTimestamp(review.FirstReviewAt),
			ReviewSubmittedAt: toNullableTimestamp(review.ReviewSubmittedAt),
			State:             review.State,
		}); err != nil {
			return fmt.Errorf("upsert pull request review: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pull request upsert transaction: %w", err)
	}

	return nil
}

func (r *Repository) ReplacePullRequestFiles(ctx context.Context, repositoryID string, githubPRID int64, files []fileChangeInput) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin file changes transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var pullRequestID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM pull_requests WHERE repository_id = $1 AND github_pr_id = $2`, parseUUID(repositoryID), githubPRID).Scan(&pullRequestID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("find pull request for file changes: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM file_changes WHERE pull_request_id = $1`, pullRequestID); err != nil {
		return fmt.Errorf("clear file changes: %w", err)
	}

	if len(files) > 0 {
		rows := make([][]any, 0, len(files))
		for _, file := range files {
			rows = append(rows, []any{
				newUUID(),
				pullRequestID,
				file.FilePath,
				file.Additions,
				file.Deletions,
				file.CommitCount,
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"file_changes"}, []string{"id", "pull_request_id", "file_path", "additions", "deletions", "commit_count"}, pgx.CopyFromRows(rows)); err != nil {
			return fmt.Errorf("copy file changes: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit file changes transaction: %w", err)
	}
	return nil
}

func (r *Repository) UpsertCommitEvents(ctx context.Context, repositoryID string, commits []commitEventInput) error {
	if len(commits) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	repoUUID := parseUUID(repositoryID)
	for _, commit := range commits {
		batch.Queue(`
		INSERT INTO commit_events (
			id, repository_id, github_commit_sha, author, author_email, message, authored_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, NOW(), NOW()
		)
		ON CONFLICT (github_commit_sha) DO UPDATE SET
			author = EXCLUDED.author,
			author_email = EXCLUDED.author_email,
			message = EXCLUDED.message,
			authored_at = EXCLUDED.authored_at,
			updated_at = NOW()`,
			newUUID(),
			repoUUID,
			commit.GitHubCommitSHA,
			commit.Author,
			nullableString(commit.AuthorEmail),
			commit.Message,
			toNullableTimestamp(&commit.AuthoredAt),
		)
	}
	return execBatch(ctx, r.db.Pool(), batch, "upsert commit events")
}

func (r *Repository) UpsertWorkflowRuns(ctx context.Context, repositoryID string, runs []workflowRunInput) error {
	if len(runs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	repoUUID := parseUUID(repositoryID)
	for _, run := range runs {
		batch.Queue(`
		INSERT INTO workflow_events (
			id, repository_id, github_workflow_run_id, workflow_name, status, conclusion, started_at, completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		)
		ON CONFLICT (github_workflow_run_id) DO UPDATE SET
			workflow_name = EXCLUDED.workflow_name,
			status = CASE
				WHEN workflow_events.completed_at IS NOT NULL
				     AND COALESCE(EXCLUDED.completed_at, EXCLUDED.started_at) < COALESCE(workflow_events.completed_at, workflow_events.started_at)
				THEN workflow_events.status
				ELSE EXCLUDED.status
			END,
			conclusion = CASE
				WHEN workflow_events.completed_at IS NOT NULL
				     AND COALESCE(EXCLUDED.completed_at, EXCLUDED.started_at) < COALESCE(workflow_events.completed_at, workflow_events.started_at)
				THEN workflow_events.conclusion
				ELSE EXCLUDED.conclusion
			END,
			started_at = COALESCE(LEAST(workflow_events.started_at, EXCLUDED.started_at), workflow_events.started_at, EXCLUDED.started_at),
			completed_at = COALESCE(GREATEST(workflow_events.completed_at, EXCLUDED.completed_at), workflow_events.completed_at, EXCLUDED.completed_at),
			updated_at = NOW()`,
			newUUID(),
			repoUUID,
			run.GitHubWorkflowRunID,
			run.WorkflowName,
			run.Status,
			nullableString(run.Conclusion),
			toNullableTimestamp(run.StartedAt),
			toNullableTimestamp(run.CompletedAt),
		)
	}
	return execBatch(ctx, r.db.Pool(), batch, "upsert workflow runs")
}

func (r *Repository) UpsertDeployments(ctx context.Context, repositoryID string, deployments []deploymentInput) error {
	if len(deployments) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	repoUUID := parseUUID(repositoryID)
	for _, deployment := range deployments {
		batch.Queue(`
		INSERT INTO deployments (
			id, repository_id, github_deployment_id, environment, status, deployed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
		ON CONFLICT (github_deployment_id) DO UPDATE SET
			environment = EXCLUDED.environment,
			status = CASE
				WHEN EXCLUDED.deployed_at >= deployments.deployed_at THEN EXCLUDED.status
				ELSE deployments.status
			END,
			deployed_at = GREATEST(deployments.deployed_at, EXCLUDED.deployed_at)`,
			newUUID(),
			repoUUID,
			deployment.GitHubDeploymentID,
			deployment.Environment,
			deployment.Status,
			toNullableTimestamp(&deployment.DeployedAt),
		)
	}
	return execBatch(ctx, r.db.Pool(), batch, "upsert deployments")
}

func execBatch(ctx context.Context, pool *pgxpool.Pool, batch *pgx.Batch, label string) error {
	results := pool.SendBatch(ctx, batch)
	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func (r *Repository) Complete(ctx context.Context, id string, repositoryID string, at time.Time) (SyncJobResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("begin sync completion transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := r.queries.WithTx(tx)
	if err := queries.UpdateRepositoryLastSyncedAt(ctx, sqlcgen.UpdateRepositoryLastSyncedAtParams{
		ID:           parseUUID(repositoryID),
		LastSyncedAt: toNullableTimestamp(&at),
	}); err != nil {
		return SyncJobResponse{}, fmt.Errorf("update repository last synced at: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE repositories
		SET initial_sync_status = 'synced',
		    initial_sync_completed_at = $2,
		    sync_error_message = NULL,
		    updated_at = NOW()
		WHERE id = $1`,
		parseUUID(repositoryID),
		toNullableTimestamp(&at),
	); err != nil {
		return SyncJobResponse{}, fmt.Errorf("update repository initial sync state: %w", err)
	}

	row, err := queries.UpdateSyncJobCompleted(ctx, sqlcgen.UpdateSyncJobCompletedParams{
		ID:         parseUUID(id),
		FinishedAt: toNullableTimestamp(&at),
	})
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark sync job completed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return SyncJobResponse{}, fmt.Errorf("commit sync completion transaction: %w", err)
	}

	return syncJobResponseFromCompletedRow(row), nil
}

func (r *Repository) GetCheckpoint(ctx context.Context, jobID string, resourceType string, key string) (*checkpointRecord, error) {
	row := r.db.Pool().QueryRow(ctx, `
		SELECT checkpoint_value, status, last_processed_at
		FROM sync_checkpoints
		WHERE sync_job_id = $1 AND resource_type = $2 AND checkpoint_key = $3`,
		parseUUID(jobID),
		resourceType,
		key,
	)

	var value pgtype.Text
	var status string
	var lastProcessedAt pgtype.Timestamptz
	if err := row.Scan(&value, &status, &lastProcessedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get sync checkpoint: %w", err)
	}

	return &checkpointRecord{
		Value:           optionalTextPtr(value),
		Status:          status,
		LastProcessedAt: optionalTimeValue(lastProcessedAt),
	}, nil
}

func (r *Repository) UpsertCheckpoint(ctx context.Context, jobID string, repositoryID string, resourceType string, key string, value *string, status string, lastProcessedAt *time.Time) error {
	if strings.TrimSpace(repositoryID) == "" {
		job, err := r.GetByID(ctx, jobID)
		if err != nil {
			return err
		}
		repositoryID = job.RepositoryID
	}

	_, err := r.db.Pool().Exec(ctx, `
		INSERT INTO sync_checkpoints (
			id, sync_job_id, repository_id, resource_type, checkpoint_key, checkpoint_value, status, last_processed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		)
		ON CONFLICT (sync_job_id, resource_type, checkpoint_key) DO UPDATE SET
			checkpoint_value = EXCLUDED.checkpoint_value,
			status = EXCLUDED.status,
			last_processed_at = EXCLUDED.last_processed_at,
			updated_at = NOW()`,
		newUUID(),
		parseUUID(jobID),
		parseUUID(repositoryID),
		resourceType,
		key,
		textPointerValue(value),
		status,
		toNullableTimestamp(lastProcessedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert sync checkpoint: %w", err)
	}
	return nil
}

func (r *Repository) getByIdempotencyKey(ctx context.Context, key string) (*SyncJobResponse, error) {
	row := r.db.Pool().QueryRow(ctx, `
		SELECT id, repository_id, status, progress, triggered_by, error_message, started_at, finished_at, created_at, updated_at
		FROM sync_jobs
		WHERE idempotency_key = $1
		LIMIT 1`, key)

	var jobID, repositoryID, triggeredBy pgtype.UUID
	var errorMessage pgtype.Text
	var startedAt, finishedAt, createdAt, updatedAt pgtype.Timestamptz
	var status string
	var progress int32
	if err := row.Scan(&jobID, &repositoryID, &status, &progress, &triggeredBy, &errorMessage, &startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get sync job by idempotency key: %w", err)
	}

	job := buildSyncJobResponse(jobID, repositoryID, status, progress, triggeredBy, errorMessage, startedAt, finishedAt, createdAt, updatedAt)
	return &job, nil
}

func buildSyncJobResponse(
	id pgtype.UUID,
	repositoryID pgtype.UUID,
	status string,
	progress int32,
	triggeredBy pgtype.UUID,
	errorMessage pgtype.Text,
	startedAt pgtype.Timestamptz,
	finishedAt pgtype.Timestamptz,
	createdAt pgtype.Timestamptz,
	updatedAt pgtype.Timestamptz,
) SyncJobResponse {
	return SyncJobResponse{
		ID:           id.String(),
		RepositoryID: repositoryID.String(),
		Status:       status,
		Progress:     int(progress),
		TriggeredBy:  optionalUUID(triggeredBy),
		ErrorMessage: optionalTextPtr(errorMessage),
		StartedAt:    optionalTimeValue(startedAt),
		FinishedAt:   optionalTimeValue(finishedAt),
		CreatedAt:    createdAt.Time.UTC(),
		UpdatedAt:    optionalTimeValue(updatedAt),
	}
}

func syncJobResponseFromCreateRow(row sqlcgen.CreateSyncJobRow) SyncJobResponse {
	return buildSyncJobResponse(row.ID, row.RepositoryID, row.Status, row.Progress, row.TriggeredBy, row.ErrorMessage, row.StartedAt, row.FinishedAt, row.CreatedAt, row.UpdatedAt)
}

func syncJobResponseFromGetRow(row sqlcgen.GetSyncJobByIDRow) SyncJobResponse {
	return buildSyncJobResponse(row.ID, row.RepositoryID, row.Status, row.Progress, row.TriggeredBy, row.ErrorMessage, row.StartedAt, row.FinishedAt, row.CreatedAt, row.UpdatedAt)
}

func syncJobResponseFromListRow(row sqlcgen.ListSyncJobsByRepositoryRow) SyncJobResponse {
	return buildSyncJobResponse(row.ID, row.RepositoryID, row.Status, row.Progress, row.TriggeredBy, row.ErrorMessage, row.StartedAt, row.FinishedAt, row.CreatedAt, row.UpdatedAt)
}

func syncJobResponseFromRunningRow(row sqlcgen.UpdateSyncJobRunningRow) SyncJobResponse {
	return buildSyncJobResponse(row.ID, row.RepositoryID, row.Status, row.Progress, row.TriggeredBy, row.ErrorMessage, row.StartedAt, row.FinishedAt, row.CreatedAt, row.UpdatedAt)
}

func syncJobResponseFromProgressRow(row sqlcgen.UpdateSyncJobProgressRow) SyncJobResponse {
	return buildSyncJobResponse(row.ID, row.RepositoryID, row.Status, row.Progress, row.TriggeredBy, row.ErrorMessage, row.StartedAt, row.FinishedAt, row.CreatedAt, row.UpdatedAt)
}

func syncJobResponseFromFailedRow(row sqlcgen.UpdateSyncJobFailedRow) SyncJobResponse {
	return buildSyncJobResponse(row.ID, row.RepositoryID, row.Status, row.Progress, row.TriggeredBy, row.ErrorMessage, row.StartedAt, row.FinishedAt, row.CreatedAt, row.UpdatedAt)
}

func syncJobResponseFromCompletedRow(row sqlcgen.UpdateSyncJobCompletedRow) SyncJobResponse {
	return buildSyncJobResponse(row.ID, row.RepositoryID, row.Status, row.Progress, row.TriggeredBy, row.ErrorMessage, row.StartedAt, row.FinishedAt, row.CreatedAt, row.UpdatedAt)
}

func newUUID() pgtype.UUID {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	return pgtype.UUID{Bytes: bytes, Valid: true}
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func nullableUUID(value *string) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return parseUUID(*value)
}

func textPointerValue(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func optionalTextPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func optionalUUID(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	result := value.String()
	return &result
}

func toNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func optionalTimeValue(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}
