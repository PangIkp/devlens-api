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

func TestGetInstallationDoesNotCountInactiveLinkedRepositoriesIntegration(t *testing.T) {
	t.Parallel()

	repo, db := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().UnixNano()
	orgID := newIntegrationUUID()
	installationRowID := newIntegrationUUID()
	activeRepositoryID := newIntegrationUUID()
	inactiveRepositoryID := newIntegrationUUID()
	inaccessibleRepositoryID := newIntegrationUUID()
	activeInstallationRepoID := newIntegrationUUID()
	inactiveInstallationRepoID := newIntegrationUUID()
	inaccessibleInstallationRepoID := newIntegrationUUID()
	unlinkedInstallationRepoID := newIntegrationUUID()
	installationID := suffix

	mustIntegrationExec(t, ctx, db, `INSERT INTO organizations (id, github_id, name, slug, created_at, deleted_at) VALUES ($1, $2, $3, $4, NOW(), NULL)`,
		orgID, suffix, "Integration Count Org", fmt.Sprintf("integration-githubconn-count-%d", suffix))

	mustIntegrationExec(t, ctx, db, `
		INSERT INTO github_installations (id, organization_id, installation_id, account_login, account_type, target_type, status, permissions_json, installed_at, updated_at)
		VALUES ($1, $2, $3, 'octocat', 'Organization', 'selected_repositories', $4, '{}'::jsonb, NOW(), NOW())`,
		installationRowID, orgID, installationID, StateConnected)

	mustIntegrationExec(t, ctx, db, `INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active) VALUES ($1, $2, $3, 'active-repo', 'integration/active-repo', 'main', TRUE)`,
		activeRepositoryID, orgID, suffix+1)
	mustIntegrationExec(t, ctx, db, `INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active) VALUES ($1, $2, $3, 'inactive-repo', 'integration/inactive-repo', 'main', FALSE)`,
		inactiveRepositoryID, orgID, suffix+2)
	mustIntegrationExec(t, ctx, db, `INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active) VALUES ($1, $2, $3, 'permission-missing-repo', 'integration/permission-missing-repo', 'main', TRUE)`,
		inaccessibleRepositoryID, orgID, suffix+4)

	mustIntegrationExec(t, ctx, db, `
		INSERT INTO github_installation_repositories (id, github_installation_id, github_repository_id, name, owner_login, full_name, installation_status, selection_status, linked_repository_id, created_at, updated_at)
		VALUES ($1, $2, $3, 'active-repo', 'octocat', 'integration/active-repo', $4, $5, $6, NOW(), NOW()),
		       ($7, $2, $8, 'inactive-repo', 'octocat', 'integration/inactive-repo', $4, $5, $9, NOW(), NOW()),
		       ($10, $2, $11, 'stale-selected-repo', 'octocat', 'integration/stale-selected-repo', $4, $5, NULL, NOW(), NOW()),
		       ($12, $2, $13, 'permission-missing-repo', 'octocat', 'integration/permission-missing-repo', $14, $5, $15, NOW(), NOW())`,
		activeInstallationRepoID, installationRowID, suffix+1, InstallationStatusAccessible, SelectionStatusSelected, activeRepositoryID,
		inactiveInstallationRepoID, suffix+2, inactiveRepositoryID,
		unlinkedInstallationRepoID, suffix+3,
		inaccessibleInstallationRepoID, suffix+4, InstallationStatusPermissionMissing, inaccessibleRepositoryID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	installation, err := repo.GetInstallation(ctx, orgID.String())
	if err != nil {
		t.Fatalf("get installation: %v", err)
	}
	if installation == nil {
		t.Fatal("expected installation")
	}
	if installation.ConnectedRepositories != 1 {
		t.Fatalf("expected 1 connected repository, got %d", installation.ConnectedRepositories)
	}

	items, err := repo.ListAccessibleRepositories(ctx, ListAccessibleRepositoriesParams{OrganizationID: orgID.String(), Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list accessible repositories: %v", err)
	}
	statuses := make(map[string]string, len(items.Items))
	for _, item := range items.Items {
		statuses[item.FullName] = item.SelectionStatus
	}
	if statuses["integration/active-repo"] != SelectionStatusSelected {
		t.Fatalf("expected active repo selected, got %q", statuses["integration/active-repo"])
	}
	if statuses["integration/inactive-repo"] != SelectionStatusNotSelected {
		t.Fatalf("expected inactive repo not selected, got %q", statuses["integration/inactive-repo"])
	}
	if statuses["integration/stale-selected-repo"] != SelectionStatusNotSelected {
		t.Fatalf("expected unlinked repo not selected, got %q", statuses["integration/stale-selected-repo"])
	}
	if statuses["integration/permission-missing-repo"] != SelectionStatusNotSelected {
		t.Fatalf("expected permission-missing repo not selected, got %q", statuses["integration/permission-missing-repo"])
	}
}

func TestRepositorySelectionReactivatesExistingRepositoryIntegration(t *testing.T) {
	t.Parallel()

	repo, db := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().UnixNano()
	orgID := newIntegrationUUID()
	installationRowID := newIntegrationUUID()
	repositoryID := newIntegrationUUID()
	installationRepoID := newIntegrationUUID()

	mustIntegrationExec(t, ctx, db, `INSERT INTO organizations (id, github_id, name, slug, created_at, deleted_at) VALUES ($1, $2, $3, $4, NOW(), NULL)`,
		orgID, suffix, "Integration Reactivate Org", fmt.Sprintf("integration-githubconn-reactivate-%d", suffix))
	mustIntegrationExec(t, ctx, db, `
		INSERT INTO github_installations (id, organization_id, installation_id, account_login, account_type, target_type, status, permissions_json, installed_at, updated_at)
		VALUES ($1, $2, $3, 'octocat', 'Organization', 'selected_repositories', $4, '{}'::jsonb, NOW(), NOW())`,
		installationRowID, orgID, suffix, StateConnected)
	mustIntegrationExec(t, ctx, db, `INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active) VALUES ($1, $2, $3, 'reactivate-repo', 'integration/reactivate-repo', 'main', FALSE)`,
		repositoryID, orgID, suffix+1)
	mustIntegrationExec(t, ctx, db, `
		INSERT INTO github_installation_repositories (id, github_installation_id, github_repository_id, name, owner_login, full_name, installation_status, selection_status, linked_repository_id, created_at, updated_at)
		VALUES ($1, $2, $3, 'reactivate-repo', 'octocat', 'integration/reactivate-repo', $4, $5, $6, NOW(), NOW())`,
		installationRepoID, installationRowID, suffix+1, InstallationStatusAccessible, SelectionStatusNotSelected, repositoryID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, orgID)
	})

	linkedID, err := repo.LinkRepositoryAndMarkSelected(ctx, orgID.String(), suffix+1, false)
	if err != nil {
		t.Fatalf("link repository: %v", err)
	}
	if linkedID != repositoryID.String() {
		t.Fatalf("expected repository id %q, got %q", repositoryID.String(), linkedID)
	}

	var isActive bool
	if err := db.Pool().QueryRow(ctx, `SELECT is_active FROM repositories WHERE id = $1`, repositoryID).Scan(&isActive); err != nil {
		t.Fatalf("read repository is_active: %v", err)
	}
	if !isActive {
		t.Fatal("expected selected repository to be reactivated")
	}

	foundInstallationID, err := repo.FindInstallationIDByRepositoryFullName(ctx, "integration/reactivate-repo")
	if err != nil {
		t.Fatalf("find installation id: %v", err)
	}
	if foundInstallationID == nil || *foundInstallationID != suffix {
		t.Fatalf("expected installation id %d, got %v", suffix, foundInstallationID)
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
