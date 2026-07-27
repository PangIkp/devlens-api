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
	row, err := r.queries.CreateSyncJob(ctx, sqlcgen.CreateSyncJobParams{
		ID:           newUUID(),
		RepositoryID: parseUUID(params.RepositoryID),
		Status:       StatusPending,
		Progress:     0,
		TriggeredBy:  nullableUUID(params.TriggeredBy),
		ErrorMessage: pgtype.Text{},
		StartedAt:    pgtype.Timestamptz{},
		FinishedAt:   pgtype.Timestamptz{},
	})
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("create sync job: %w", err)
	}
	return syncJobResponseFromCreateRow(row), nil
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
	row, err := r.queries.UpdateSyncJobFailed(ctx, sqlcgen.UpdateSyncJobFailedParams{
		ID:           parseUUID(id),
		ErrorMessage: pgtype.Text{String: message, Valid: true},
		FinishedAt:   toNullableTimestamp(&at),
	})
	if err != nil {
		return SyncJobResponse{}, fmt.Errorf("mark sync job failed: %w", err)
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
