package githubconnection

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDisconnectInstallationDeactivatesLinkedRepositoriesIntegration(t *testing.T) {
	t.Parallel()

	repo, db := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().UnixNano()
	orgID := newIntegrationUUID()
	installationRowID := newIntegrationUUID()
	repositoryID := newIntegrationUUID()
	installationRepoID := newIntegrationUUID()
	installationID := suffix

	mustIntegrationExec(t, ctx, db, `INSERT INTO organizations (id, github_id, name, slug, created_at, deleted_at) VALUES ($1, $2, $3, $4, NOW(), NULL)`,
		orgID, suffix, "Integration GitHub Org", fmt.Sprintf("integration-githubconn-org-%d", suffix))

	mustIntegrationExec(t, ctx, db, `
		INSERT INTO github_installations (id, organization_id, installation_id, account_login, account_type, target_type, status, permissions_json, installed_at, updated_at)
		VALUES ($1, $2, $3, 'octocat', 'Organization', 'selected_repositories', $4, '{}'::jsonb, NOW(), NOW())`,
		installationRowID, orgID, installationID, StateConnected)

	mustIntegrationExec(t, ctx, db, `INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active) VALUES ($1, $2, $3, $4, $5, $6, TRUE)`,
		repositoryID, orgID, suffix, "disconnect-repo", "integration/disconnect-repo", "main")

	mustIntegrationExec(t, ctx, db, `
		INSERT INTO github_installation_repositories (id, github_installation_id, github_repository_id, name, owner_login, full_name, installation_status, selection_status, linked_repository_id, created_at, updated_at)
		VALUES ($1, $2, $3, 'disconnect-repo', 'octocat', 'integration/disconnect-repo', $4, $5, $6, NOW(), NOW())`,
		installationRepoID, installationRowID, suffix, InstallationStatusAccessible, SelectionStatusSelected, repositoryID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	disconnectedAt := time.Now().UTC()
	if err := repo.DisconnectInstallation(ctx, installationID, StateInstallationRequired, &disconnectedAt); err != nil {
		t.Fatalf("disconnect installation: %v", err)
	}

	var isActive bool
	if err := db.Pool().QueryRow(ctx, `SELECT is_active FROM repositories WHERE id = $1`, repositoryID).Scan(&isActive); err != nil {
		t.Fatalf("read repository is_active: %v", err)
	}
	if isActive {
		t.Fatal("expected repository to be deactivated after installation disconnect")
	}

	var selectionStatus string
	var linkedRepositoryID pgtype.UUID
	if err := db.Pool().QueryRow(ctx, `SELECT selection_status, linked_repository_id FROM github_installation_repositories WHERE id = $1`, installationRepoID).
		Scan(&selectionStatus, &linkedRepositoryID); err != nil {
		t.Fatalf("read installation repository: %v", err)
	}
	if selectionStatus != SelectionStatusNotSelected {
		t.Fatalf("expected selection status %q, got %q", SelectionStatusNotSelected, selectionStatus)
	}
	if linkedRepositoryID.Valid {
		t.Fatal("expected linked_repository_id to be cleared")
	}

	var status string
	var storedDisconnectedAt pgtype.Timestamptz
	if err := db.Pool().QueryRow(ctx, `SELECT status, disconnected_at FROM github_installations WHERE id = $1`, installationRowID).
		Scan(&status, &storedDisconnectedAt); err != nil {
		t.Fatalf("read installation: %v", err)
	}
	if status != StateInstallationRequired {
		t.Fatalf("expected installation status %q, got %q", StateInstallationRequired, status)
	}
	if !storedDisconnectedAt.Valid {
		t.Fatal("expected disconnected_at to be set")
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

func mustIntegrationExec(t *testing.T, ctx context.Context, db *postgres.DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Pool().Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

var integrationUUIDCounter atomic.Uint64

func newIntegrationUUID() pgtype.UUID {
	var value pgtype.UUID
	value.Valid = true
	binary.BigEndian.PutUint64(value.Bytes[0:8], uint64(time.Now().UTC().UnixNano()))
	binary.BigEndian.PutUint64(value.Bytes[8:16], integrationUUIDCounter.Add(1))
	return value
}
