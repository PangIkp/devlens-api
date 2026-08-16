package pullrequest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrPullRequestNotFound = errors.New("pull request not found")
	ErrRepositoryNotFound  = errors.New("repository not found")
)

type Repository struct {
	db *postgres.DB
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureRepositoryExists(ctx context.Context, repositoryID string) error {
	var exists bool
	err := r.db.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM repositories WHERE id = $1)`, parseUUID(repositoryID)).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check repository exists: %w", err)
	}
	if !exists {
		return ErrRepositoryNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, params ListParams) (ListResult, error) {
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
WHERE pr.repository_id = $1
  AND ($2 = '' OR pr.state = $2)
  AND ($3 = '' OR pr.title ILIKE '%' || $3 || '%' OR pr.author ILIKE '%' || $3 || '%')
ORDER BY
  CASE WHEN $4 = 'number' AND $5 = 'asc' THEN pr.number END ASC,
  CASE WHEN $4 = 'number' AND $5 = 'desc' THEN pr.number END DESC,
  CASE WHEN $4 = 'createdAt' AND $5 = 'asc' THEN pr.created_at END ASC,
  CASE WHEN $4 = 'createdAt' AND $5 = 'desc' THEN pr.created_at END DESC,
  pr.created_at DESC
LIMIT $6 OFFSET $7`

	rows, err := r.db.Pool().Query(ctx, query, parseUUID(params.RepositoryID), params.Status, params.Search, params.SortBy, params.SortOrder, params.PageSize, (params.Page-1)*params.PageSize)
	if err != nil {
		return ListResult{}, fmt.Errorf("list pull requests: %w", err)
	}
	defer rows.Close()

	result := ListResult{Items: make([]ListItem, 0)}
	for rows.Next() {
		var item ListItem
		var mergedAt, closedAt pgtype.Timestamptz
		if err := rows.Scan(
			&item.ID,
			&item.Repository.ID,
			&item.Repository.FullName,
			&item.GithubPRID,
			&item.Number,
			&item.Title,
			&item.Author,
			&item.State,
			&item.CreatedAt,
			&mergedAt,
			&closedAt,
			&item.Additions,
			&item.Deletions,
			&item.FilesChanged,
			&item.IsDraft,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan pull request list item: %w", err)
		}
		item.CreatedAt = item.CreatedAt.UTC()
		item.MergedAt = optionalTime(mergedAt)
		item.ClosedAt = optionalTime(closedAt)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}

	countQuery := `
SELECT COUNT(*)
FROM pull_requests
WHERE repository_id = $1
  AND ($2 = '' OR state = $2)
  AND ($3 = '' OR title ILIKE '%' || $3 || '%' OR author ILIKE '%' || $3 || '%')`
	if err := r.db.Pool().QueryRow(ctx, countQuery, parseUUID(params.RepositoryID), params.Status, params.Search).Scan(&result.TotalItems); err != nil {
		return ListResult{}, fmt.Errorf("count pull requests: %w", err)
	}
	return result, nil
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
	response.Timeline = buildTimeline(response.Author, response.CreatedAt, response.MergedAt, response.ClosedAt, reviews)
	response.RiskIndicator = buildRiskIndicator(response, reviews, changes)
	response.CycleTimeMinutes = computeCycleTimeMinutes(response.CreatedAt, response.MergedAt)
	response.ReviewWaitMinutes = computeReviewWaitMinutes(reviews)
	return response, nil
}

func computeCycleTimeMinutes(createdAt time.Time, mergedAt *time.Time) *int64 {
	if mergedAt == nil || mergedAt.Before(createdAt) {
		return nil
	}
	minutes := int64(mergedAt.Sub(createdAt).Minutes())
	return &minutes
}

func computeReviewWaitMinutes(reviews []Review) *int64 {
	var earliest *time.Time
	var waitMinutes int64

	for _, review := range reviews {
		if isBotReviewer(review.Reviewer) {
			continue
		}
		if review.ReviewRequestedAt == nil || review.FirstReviewAt == nil {
			continue
		}
		if review.FirstReviewAt.Before(*review.ReviewRequestedAt) {
			continue
		}
		if earliest != nil && !review.FirstReviewAt.Before(*earliest) {
			continue
		}
		firstReviewAt := *review.FirstReviewAt
		earliest = &firstReviewAt
		waitMinutes = int64(review.FirstReviewAt.Sub(*review.ReviewRequestedAt).Minutes())
	}

	if earliest == nil {
		return nil
	}
	return &waitMinutes
}

func isBotReviewer(reviewer string) bool {
	name := strings.ToLower(strings.TrimSpace(reviewer))
	if name == "" {
		return true
	}
	switch {
	case strings.HasSuffix(name, "[bot]"):
		return true
	case strings.HasPrefix(name, "dependabot"):
		return true
	case strings.HasSuffix(name, "-bot"):
		return true
	case name == "github-actions":
		return true
	case name == "web-flow":
		return true
	default:
		return false
	}
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

func buildTimeline(author string, createdAt time.Time, mergedAt, closedAt *time.Time, reviews []Review) []TimelineEvent {
	events := make([]TimelineEvent, 0, 2+len(reviews)*3)
	authorCopy := author
	events = append(events, TimelineEvent{
		Type:       "created",
		Label:      "Pull request created",
		Actor:      &authorCopy,
		OccurredAt: createdAt.UTC(),
	})

	for _, review := range reviews {
		reviewer := review.Reviewer
		if review.ReviewRequestedAt != nil {
			events = append(events, TimelineEvent{
				Type:       "review_requested",
				Label:      "Review requested",
				Actor:      &reviewer,
				OccurredAt: review.ReviewRequestedAt.UTC(),
			})
		}
		if review.FirstReviewAt != nil {
			events = append(events, TimelineEvent{
				Type:       "review_started",
				Label:      "Review started",
				Actor:      &reviewer,
				OccurredAt: review.FirstReviewAt.UTC(),
			})
		}
		if review.ReviewSubmittedAt != nil {
			state := review.State
			events = append(events, TimelineEvent{
				Type:       "review_submitted",
				Label:      "Review submitted",
				Actor:      &reviewer,
				State:      &state,
				OccurredAt: review.ReviewSubmittedAt.UTC(),
			})
		}
	}

	if mergedAt != nil {
		events = append(events, TimelineEvent{
			Type:       "merged",
			Label:      "Pull request merged",
			OccurredAt: mergedAt.UTC(),
		})
	} else if closedAt != nil {
		events = append(events, TimelineEvent{
			Type:       "closed",
			Label:      "Pull request closed",
			OccurredAt: closedAt.UTC(),
		})
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return timelinePriority(events[i].Type) < timelinePriority(events[j].Type)
		}
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})

	return events
}

func timelinePriority(eventType string) int {
	switch eventType {
	case "created":
		return 0
	case "review_requested":
		return 1
	case "review_started":
		return 2
	case "review_submitted":
		return 3
	case "merged":
		return 4
	case "closed":
		return 5
	default:
		return 100
	}
}

func buildRiskIndicator(pr Response, reviews []Review, changes []FileChange) RiskIndicator {
	reasons := make([]string, 0, 4)
	levelScore := 0

	totalChangedLines := pr.Additions + pr.Deletions
	if totalChangedLines >= 1000 {
		levelScore += 2
		reasons = append(reasons, "very_large_change")
	} else if totalChangedLines >= 400 {
		levelScore++
		reasons = append(reasons, "large_change")
	}

	if pr.FilesChanged >= 25 {
		levelScore += 2
		reasons = append(reasons, "many_files_changed")
	} else if pr.FilesChanged >= 10 {
		levelScore++
		reasons = append(reasons, "moderate_file_spread")
	}

	if len(reviews) == 0 && !pr.IsDraft {
		levelScore += 2
		reasons = append(reasons, "no_reviews_recorded")
	}

	maxFileCommitCount := 0
	for _, change := range changes {
		if change.CommitCount > maxFileCommitCount {
			maxFileCommitCount = change.CommitCount
		}
	}
	if maxFileCommitCount >= 10 {
		levelScore++
		reasons = append(reasons, "high_file_rework")
	}

	level := "low"
	switch {
	case levelScore >= 4:
		level = "high"
	case levelScore >= 2:
		level = "medium"
	}

	return RiskIndicator{
		Level:   level,
		Reasons: reasons,
	}
}
