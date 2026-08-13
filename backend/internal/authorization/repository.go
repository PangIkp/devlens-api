package authorization

import (
	"context"
	"errors"
	"fmt"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrForbidden               = errors.New("forbidden")
	ErrOrganizationNotFound    = errors.New("organization not found")
	ErrRepositoryNotFound      = errors.New("repository not found")
	ErrPullRequestNotFound     = errors.New("pull request not found")
	ErrSyncJobNotFound         = errors.New("sync job not found")
	ErrWebhookDeliveryNotFound = errors.New("webhook delivery not found")
)

type Repository struct {
	db *postgres.DB
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetOrganizationRole(ctx context.Context, userID string, organizationID string) (string, error) {
	return r.getRole(
		ctx,
		`SELECT role
FROM organization_members
WHERE organization_id = $1 AND user_id = $2`,
		`SELECT EXISTS(
SELECT 1
FROM organizations
WHERE id = $1 AND deleted_at IS NULL
)`,
		ErrOrganizationNotFound,
		organizationID,
		userID,
	)
}

func (r *Repository) GetRepositoryRole(ctx context.Context, userID string, repositoryID string) (string, error) {
	return r.getRole(
		ctx,
		`SELECT om.role
FROM repositories repo
INNER JOIN organization_members om ON om.organization_id = repo.organization_id
WHERE repo.id = $1 AND om.user_id = $2`,
		`SELECT EXISTS(
SELECT 1
FROM repositories
WHERE id = $1
)`,
		ErrRepositoryNotFound,
		repositoryID,
		userID,
	)
}

func (r *Repository) GetPullRequestRole(ctx context.Context, userID string, pullRequestID string) (string, error) {
	return r.getRole(
		ctx,
		`SELECT om.role
FROM pull_requests pr
INNER JOIN repositories repo ON repo.id = pr.repository_id
INNER JOIN organization_members om ON om.organization_id = repo.organization_id
WHERE pr.id = $1 AND om.user_id = $2`,
		`SELECT EXISTS(
SELECT 1
FROM pull_requests
WHERE id = $1
)`,
		ErrPullRequestNotFound,
		pullRequestID,
		userID,
	)
}

func (r *Repository) GetSyncJobRole(ctx context.Context, userID string, syncJobID string) (string, error) {
	return r.getRole(
		ctx,
		`SELECT om.role
FROM sync_jobs sj
INNER JOIN repositories repo ON repo.id = sj.repository_id
INNER JOIN organization_members om ON om.organization_id = repo.organization_id
WHERE sj.id = $1 AND om.user_id = $2`,
		`SELECT EXISTS(
SELECT 1
FROM sync_jobs
WHERE id = $1
)`,
		ErrSyncJobNotFound,
		syncJobID,
		userID,
	)
}

func (r *Repository) GetWebhookDeliveryRole(ctx context.Context, userID string, deliveryID string) (string, error) {
	return r.getRole(
		ctx,
		`SELECT om.role
FROM webhook_deliveries wd
LEFT JOIN repositories repo ON repo.id = wd.repository_id
LEFT JOIN github_installations gi ON gi.id = wd.installation_id
INNER JOIN organization_members om ON om.organization_id = COALESCE(repo.organization_id, gi.organization_id)
WHERE wd.id = $1 AND om.user_id = $2`,
		`SELECT EXISTS(
SELECT 1
FROM webhook_deliveries
WHERE id = $1
)`,
		ErrWebhookDeliveryNotFound,
		deliveryID,
		userID,
	)
}

func (r *Repository) getRole(ctx context.Context, roleQuery string, existsQuery string, notFound error, resourceID string, userID string) (string, error) {
	var role string
	err := r.db.Pool().QueryRow(ctx, roleQuery, parseUUID(resourceID), parseUUID(userID)).Scan(&role)
	if err == nil {
		return role, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("load access role: %w", err)
	}

	var exists bool
	if err := r.db.Pool().QueryRow(ctx, existsQuery, parseUUID(resourceID)).Scan(&exists); err != nil {
		return "", fmt.Errorf("check resource exists: %w", err)
	}
	if !exists {
		return "", notFound
	}

	return "", ErrForbidden
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}
