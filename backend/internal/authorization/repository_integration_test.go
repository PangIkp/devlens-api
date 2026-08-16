package authorization

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestGetRoleRevokedAfterOrganizationSoftDeleteIntegration(t *testing.T) {
	t.Parallel()

	repo, db := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().UnixNano()
	userID := newUUID()
	orgID := newUUID()
	memberID := newUUID()
	repositoryID := newUUID()
	pullRequestID := newUUID()
	syncJobID := newUUID()
	installationID := newUUID()
	webhookDeliveryID := newUUID()

	mustExec(t, ctx, db, `INSERT INTO users (id, email, name, created_at) VALUES ($1, $2, $3, NOW())`,
		userID, fmt.Sprintf("integration-authz-%d@example.com", suffix), "Integration Authz User")

	mustExec(t, ctx, db, `INSERT INTO organizations (id, github_id, name, slug, created_at, deleted_at) VALUES ($1, $2, $3, $4, NOW(), NULL)`,
		orgID, suffix, "Integration Authz Org", fmt.Sprintf("integration-authz-org-%d", suffix))

	mustExec(t, ctx, db, `INSERT INTO organization_members (id, organization_id, user_id, role) VALUES ($1, $2, $3, $4)`,
		memberID, orgID, userID, "owner")

	mustExec(t, ctx, db, `INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active) VALUES ($1, $2, $3, $4, $5, $6, TRUE)`,
		repositoryID, orgID, suffix, "authz-repo", "integration/authz-repo", "main")

	mustExec(t, ctx, db, `INSERT INTO pull_requests (id, repository_id, github_pr_id, number, title, author, state, created_at) VALUES ($1, $2, $3, 1, 'Integration PR', 'octocat', 'open', NOW())`,
		pullRequestID, repositoryID, suffix)

	mustExec(t, ctx, db, `INSERT INTO sync_jobs (id, repository_id, status, progress) VALUES ($1, $2, 'pending', 0)`,
		syncJobID, repositoryID)

	mustExec(t, ctx, db, `INSERT INTO github_installations (id, organization_id, installation_id, installed_at) VALUES ($1, $2, $3, NOW())`,
		installationID, orgID, suffix)

	mustExec(t, ctx, db, `INSERT INTO webhook_deliveries (id, repository_id, github_delivery_id, event_type, processed, received_at) VALUES ($1, $2, $3, 'push', FALSE, NOW())`,
		webhookDeliveryID, repositoryID, fmt.Sprintf("integration-authz-delivery-%d", suffix))

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	userIDStr := formatUUID(userID)
	repositoryIDStr := formatUUID(repositoryID)
	pullRequestIDStr := formatUUID(pullRequestID)
	syncJobIDStr := formatUUID(syncJobID)
	webhookDeliveryIDStr := formatUUID(webhookDeliveryID)

	orgIDStr := formatUUID(orgID)

	if role, err := repo.GetOrganizationRole(ctx, userIDStr, orgIDStr); err != nil || role != "owner" {
		t.Fatalf("expected owner role for organization before soft delete, got role=%q err=%v", role, err)
	}
	if role, err := repo.GetRepositoryRole(ctx, userIDStr, repositoryIDStr); err != nil || role != "owner" {
		t.Fatalf("expected owner role for repository before soft delete, got role=%q err=%v", role, err)
	}
	if role, err := repo.GetPullRequestRole(ctx, userIDStr, pullRequestIDStr); err != nil || role != "owner" {
		t.Fatalf("expected owner role for pull request before soft delete, got role=%q err=%v", role, err)
	}
	if role, err := repo.GetSyncJobRole(ctx, userIDStr, syncJobIDStr); err != nil || role != "owner" {
		t.Fatalf("expected owner role for sync job before soft delete, got role=%q err=%v", role, err)
	}
	if role, err := repo.GetWebhookDeliveryRole(ctx, userIDStr, webhookDeliveryIDStr); err != nil || role != "owner" {
		t.Fatalf("expected owner role for webhook delivery before soft delete, got role=%q err=%v", role, err)
	}

	mustExec(t, ctx, db, `UPDATE organizations SET deleted_at = NOW() WHERE id = $1`, orgID)

	if _, err := repo.GetOrganizationRole(ctx, userIDStr, orgIDStr); !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound after org soft delete, got %v", err)
	}
	if _, err := repo.GetRepositoryRole(ctx, userIDStr, repositoryIDStr); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("expected ErrRepositoryNotFound after org soft delete, got %v", err)
	}
	if _, err := repo.GetPullRequestRole(ctx, userIDStr, pullRequestIDStr); !errors.Is(err, ErrPullRequestNotFound) {
		t.Fatalf("expected ErrPullRequestNotFound after org soft delete, got %v", err)
	}
	if _, err := repo.GetSyncJobRole(ctx, userIDStr, syncJobIDStr); !errors.Is(err, ErrSyncJobNotFound) {
		t.Fatalf("expected ErrSyncJobNotFound after org soft delete, got %v", err)
	}
	if _, err := repo.GetWebhookDeliveryRole(ctx, userIDStr, webhookDeliveryIDStr); !errors.Is(err, ErrWebhookDeliveryNotFound) {
		t.Fatalf("expected ErrWebhookDeliveryNotFound after org soft delete, got %v", err)
	}
}

func openIntegrationRepository(t *testing.T) (*Repository, *postgres.DB) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Postgres.ConnectTimeout)
	defer cancel()

	db, err := postgres.Open(ctx, cfg.Postgres, nil)
	if err != nil {
		t.Skipf("skip integration test: postgres unavailable: %v", err)
	}
	t.Cleanup(db.Close)

	return NewRepository(db), db
}

func mustExec(t *testing.T, ctx context.Context, db *postgres.DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Pool().Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

var uuidCounter atomic.Uint64

func newUUID() pgtype.UUID {
	var value pgtype.UUID
	value.Valid = true
	binary.BigEndian.PutUint64(value.Bytes[0:8], uint64(time.Now().UTC().UnixNano()))
	binary.BigEndian.PutUint64(value.Bytes[8:16], uuidCounter.Add(1))
	return value
}

func formatUUID(value pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value.Bytes[0:4], value.Bytes[4:6], value.Bytes[6:8], value.Bytes[8:10], value.Bytes[10:16])
}
