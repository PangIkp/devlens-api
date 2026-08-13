package datalifecycle

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCleanupExpiredWebhookPayloadsIntegration(t *testing.T) {
	t.Parallel()

	db, cfg := openIntegrationPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	graph := seedLifecycleGraph(t, ctx, db, false)
	service := NewService(db, nil, cfg.DataLifecycle)

	cleaned, err := service.CleanupExpiredWebhookPayloads(ctx)
	if err != nil {
		t.Fatalf("cleanup expired webhook payloads: %v", err)
	}
	if cleaned < 1 {
		t.Fatalf("expected at least one cleaned payload, got %d", cleaned)
	}

	var payloadValid bool
	if err := db.Pool().QueryRow(ctx, `
		SELECT payload IS NOT NULL
		FROM webhook_deliveries
		WHERE github_delivery_id = $1`, graph.deliveryID).Scan(&payloadValid); err != nil {
		t.Fatalf("load webhook delivery payload: %v", err)
	}
	if payloadValid {
		t.Fatal("expected expired payload to be cleared")
	}
}

func TestPurgeExpiredDisconnectedInstallationsIntegration(t *testing.T) {
	t.Parallel()

	db, cfg := openIntegrationPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	graph := seedLifecycleGraph(t, ctx, db, true)
	service := NewService(db, nil, cfg.DataLifecycle)
	service.now = func() time.Time { return graph.now }

	deletedInstallations, purgedRepos, err := service.PurgeExpiredDisconnectedInstallations(ctx)
	if err != nil {
		t.Fatalf("purge expired disconnected installations: %v", err)
	}
	if deletedInstallations != 1 {
		t.Fatalf("expected 1 deleted installation, got %d", deletedInstallations)
	}
	if purgedRepos != 1 {
		t.Fatalf("expected 1 purged analytics repo, got %d", purgedRepos)
	}

	assertCountZero(t, ctx, db, "SELECT COUNT(*) FROM github_installations WHERE id = $1", graph.installationUUID)
	assertCountZero(t, ctx, db, "SELECT COUNT(*) FROM repositories WHERE id = $1", graph.repositoryUUID)
	assertCountZero(t, ctx, db, "SELECT COUNT(*) FROM webhook_deliveries WHERE github_delivery_id = $1", graph.deliveryID)
}

type lifecycleGraph struct {
	now              time.Time
	installationUUID pgtype.UUID
	repositoryUUID   pgtype.UUID
	deliveryID       string
}

func openIntegrationPostgres(t *testing.T) (*postgres.DB, config.Config) {
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

	return db, cfg
}

func seedLifecycleGraph(t *testing.T, ctx context.Context, db *postgres.DB, disconnected bool) lifecycleGraph {
	t.Helper()

	now := time.Now().UTC()
	suffix := now.UnixNano()

	orgUUID := newUUID()
	installationUUID := newUUID()
	installationRepoUUID := newUUID()
	repositoryUUID := newUUID()
	deliveryID := fmt.Sprintf("lifecycle-delivery-%d", suffix)

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO organizations (id, github_id, name, slug, created_at, deleted_at)
		VALUES ($1, $2, $3, $4, NOW(), NULL)`,
		orgUUID,
		suffix,
		"Lifecycle Org",
		fmt.Sprintf("lifecycle-%d", suffix),
	); err != nil {
		t.Fatalf("insert organization: %v", err)
	}

	var disconnectedAt any
	if disconnected {
		disconnectedAt = now.Add(-45 * 24 * time.Hour)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO github_installations (
			id, organization_id, installation_id, installed_at, account_login, account_type, target_type,
			status, permissions_json, installed_by_github_user_id, updated_at, disconnected_at
		) VALUES (
			$1, $2, $3, NOW(), 'pangikp', 'User', 'account', 'installation_required',
			'{}'::jsonb, $4, NOW(), $5
		)`,
		installationUUID,
		orgUUID,
		suffix,
		suffix,
		disconnectedAt,
	); err != nil {
		t.Fatalf("insert installation: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO github_installation_repositories (
			id, github_installation_id, github_repository_id, name, owner_login, full_name, private,
			default_branch, installation_status, selection_status, permissions_json, linked_repository_id, last_discovered_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, FALSE, $7, 'accessible', 'selected', '{}'::jsonb, NULL, NOW(), NOW(), NOW()
		)`,
		installationRepoUUID,
		installationUUID,
		suffix,
		"devlens-api",
		"pangikp",
		fmt.Sprintf("pangikp/devlens-api-%d", suffix),
		"main",
	); err != nil {
		t.Fatalf("insert installation repository: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO repositories (
			id, organization_id, github_id, name, full_name, default_branch, is_active, github_installation_repository_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, TRUE, $7
		)`,
		repositoryUUID,
		orgUUID,
		suffix,
		"devlens-api",
		fmt.Sprintf("pangikp/devlens-api-%d", suffix),
		"main",
		installationRepoUUID,
	); err != nil {
		t.Fatalf("insert repository: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		UPDATE github_installation_repositories
		SET linked_repository_id = $2
		WHERE id = $1`,
		installationRepoUUID,
		repositoryUUID,
	); err != nil {
		t.Fatalf("link installation repository: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO webhook_deliveries (
			id, repository_id, github_delivery_id, event_type, processed, received_at, action, payload,
			github_installation_id, payload_retention_until, processing_status, updated_at
		) VALUES (
			$1, $2, $3, 'push', FALSE, NOW(), 'synchronize', '{"ok":true}'::jsonb,
			$4, $5, 'received', NOW()
		)`,
		newUUID(),
		repositoryUUID,
		deliveryID,
		installationUUID,
		now.Add(-48*time.Hour),
	); err != nil {
		t.Fatalf("insert webhook delivery: %v", err)
	}

	return lifecycleGraph{
		now:              now,
		installationUUID: installationUUID,
		repositoryUUID:   repositoryUUID,
		deliveryID:       deliveryID,
	}
}

func assertCountZero(t *testing.T, ctx context.Context, db *postgres.DB, query string, arg any) {
	t.Helper()

	var count int
	if err := db.Pool().QueryRow(ctx, query, arg).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected zero rows for query %q, got %d", query, count)
	}
}

func newUUID() pgtype.UUID {
	var value pgtype.UUID
	value.Valid = true
	copy(value.Bytes[:], []byte(fmt.Sprintf("%016x%016x", time.Now().UTC().UnixNano(), time.Now().UTC().UnixNano())))
	return value
}
