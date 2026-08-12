package pullrequest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrPullRequestNotFound = errors.New("pull request not found")

type Repository struct {
	db *postgres.DB
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(ctx context.Context, id string) (Response, error) {
	query := `
SELECT pr.id::text,
       pr.repository_id::text,
       repo.full_name,
       pr.github_pr_id,
       pr.number,
       pr.title,
       pr.author,
       pr.state,
       pr.created_at,
       pr.merged_at,
       pr.closed_at,
       pr.additions,
       pr.deletions,
       pr.files_changed,
       pr.is_draft
FROM pull_requests pr
INNER JOIN repositories repo ON repo.id = pr.repository_id
WHERE pr.id = $1`

	var response Response
	var mergedAt, closedAt pgtype.Timestamptz
	err := r.db.Pool().QueryRow(ctx, query, parseUUID(id)).Scan(
		&response.ID,
		&response.Repository.ID,
		&response.Repository.FullName,
		&response.GithubPRID,
		&response.Number,
		&response.Title,
		&response.Author,
		&response.State,
		&response.CreatedAt,
		&mergedAt,
		&closedAt,
		&response.Additions,
		&response.Deletions,
		&response.FilesChanged,
		&response.IsDraft,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Response{}, ErrPullRequestNotFound
		}
		return Response{}, fmt.Errorf("get pull request: %w", err)
	}
	response.CreatedAt = response.CreatedAt.UTC()
	response.MergedAt = optionalTime(mergedAt)
	response.ClosedAt = optionalTime(closedAt)

	reviews, err := r.listReviews(ctx, id)
	if err != nil {
		return Response{}, err
	}
	changes, err := r.listFileChanges(ctx, id)
	if err != nil {
		return Response{}, err
	}
	response.Reviews = reviews
	response.FileChanges = changes
	return response, nil
}

func (r *Repository) listReviews(ctx context.Context, pullRequestID string) ([]Review, error) {
	query := `
SELECT id::text, github_review_id, reviewer, state, review_requested_at, first_review_at, review_submitted_at
FROM pull_request_reviews
WHERE pull_request_id = $1
ORDER BY COALESCE(review_submitted_at, first_review_at, review_requested_at) ASC, id ASC`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(pullRequestID))
	if err != nil {
		return nil, fmt.Errorf("list pull request reviews: %w", err)
	}
	defer rows.Close()

	items := make([]Review, 0)
	for rows.Next() {
		var item Review
		var requestedAt, firstAt, submittedAt pgtype.Timestamptz
		if err := rows.Scan(&item.ID, &item.GithubReviewID, &item.Reviewer, &item.State, &requestedAt, &firstAt, &submittedAt); err != nil {
			return nil, fmt.Errorf("scan pull request review: %w", err)
		}
		item.ReviewRequestedAt = optionalTime(requestedAt)
		item.FirstReviewAt = optionalTime(firstAt)
		item.ReviewSubmittedAt = optionalTime(submittedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) listFileChanges(ctx context.Context, pullRequestID string) ([]FileChange, error) {
	query := `
SELECT id::text, file_path, additions, deletions, commit_count
FROM file_changes
WHERE pull_request_id = $1
ORDER BY (additions + deletions) DESC, file_path ASC`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(pullRequestID))
	if err != nil {
		return nil, fmt.Errorf("list file changes: %w", err)
	}
	defer rows.Close()

	items := make([]FileChange, 0)
	for rows.Next() {
		var item FileChange
		if err := rows.Scan(&item.ID, &item.FilePath, &item.Additions, &item.Deletions, &item.CommitCount); err != nil {
			return nil, fmt.Errorf("scan file change: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}
