package datalifecycle

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
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

func TestPurgeExpiredAnalyticsRawDataIntegration(t *testing.T) {
	t.Parallel()

	pg, cfg := openIntegrationPostgres(t)
	ch := openIntegrationClickHouse(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	suffix := time.Now().UTC().UnixNano()
	orgUUID := newUUID()
	repoUUID := newUUID()
	repoIDText := formatUUID(repoUUID)

	if _, err := pg.Pool().Exec(ctx, `
		INSERT INTO organizations (id, github_id, name, slug, created_at, deleted_at)
		VALUES ($1, $2, $3, $4, NOW(), NULL)`,
		orgUUID, suffix, "Retention Org", fmt.Sprintf("retention-org-%d", suffix),
	); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgUUID)
	})

	// This organization opted into a much shorter raw-retention window than
	// the 180-day default — proving the purge honors the per-org value,
	// not a single global cutoff.
	if _, err := pg.Pool().Exec(ctx, `
		INSERT INTO organization_retention_settings (organization_id, analytics_raw_retention_days, created_at, updated_at)
		VALUES ($1, 7, NOW(), NOW())`,
		orgUUID,
	); err != nil {
		t.Fatalf("insert retention settings: %v", err)
	}

	if _, err := pg.Pool().Exec(ctx, `
		INSERT INTO repositories (id, organization_id, github_id, name, full_name, default_branch, is_active)
		VALUES ($1, $2, $3, $4, $5, 'main', TRUE)`,
		repoUUID, orgUUID, suffix, "retention-repo", fmt.Sprintf("devlens/retention-repo-%d", suffix),
	); err != nil {
		t.Fatalf("insert repository: %v", err)
	}

	now := time.Now().UTC()
	expiredSyncedAt := now.Add(-30 * 24 * time.Hour) // older than the org's 7-day window
	freshSyncedAt := now.Add(-1 * time.Hour)         // within the org's 7-day window

	type pullRequestRow struct {
		ID           string `json:"id"`
		RepositoryID string `json:"repository_id"`
		GithubPrID   int64  `json:"github_pr_id"`
		Number       int32  `json:"number"`
		Title        string `json:"title"`
		Author       string `json:"author"`
		State        string `json:"state"`
		CreatedAt    string `json:"created_at"`
		Additions    int32  `json:"additions"`
		Deletions    int32  `json:"deletions"`
		FilesChanged int32  `json:"files_changed"`
		IsDraft      bool   `json:"is_draft"`
		SyncedAt     string `json:"synced_at"`
	}

	rows := []pullRequestRow{
		{
			ID: fmt.Sprintf("expired-%d", suffix), RepositoryID: repoIDText, GithubPrID: suffix, Number: 1,
			Title: "expired", Author: "tester", State: "open", CreatedAt: expiredSyncedAt.Format(clickHouseDateTimeLayout),
			SyncedAt: expiredSyncedAt.Format(clickHouseDateTimeLayout),
		},
		{
			ID: fmt.Sprintf("fresh-%d", suffix), RepositoryID: repoIDText, GithubPrID: suffix + 1, Number: 2,
			Title: "fresh", Author: "tester", State: "open", CreatedAt: freshSyncedAt.Format(clickHouseDateTimeLayout),
			SyncedAt: freshSyncedAt.Format(clickHouseDateTimeLayout),
		},
	}
	if err := ch.InsertJSONEachRow(ctx, "INSERT INTO pull_requests", rows); err != nil {
		t.Fatalf("insert clickhouse pull requests: %v", err)
	}
	t.Cleanup(func() {
		_ = ch.Exec(context.Background(), fmt.Sprintf("ALTER TABLE pull_requests DELETE WHERE repository_id = '%s'", repoIDText))
	})

	service := NewService(pg, ch, cfg.DataLifecycle)

	// This runs against a shared dev database with other organizations
	// already in it (this is not the only one), so only assert that ours
	// was among those processed, not an exact count.
	purgedOrgs, err := service.PurgeExpiredAnalyticsRawData(ctx)
	if err != nil {
		t.Fatalf("purge expired analytics raw data: %v", err)
	}
	if purgedOrgs < 1 {
		t.Fatalf("expected at least 1 organization processed, got %d", purgedOrgs)
	}

	// The DELETE mutation applies asynchronously in ClickHouse (see the
	// comment on purgeExpiredAnalyticsRawDataForOrganization), so poll for
	// it to land instead of asserting immediately.
	type idRow struct {
		ID string `json:"id"`
	}
	var remaining []idRow
	deadline := time.Now().Add(15 * time.Second)
	for {
		remaining, err = clickhouse.QueryJSONEachRow[idRow](ctx, ch, fmt.Sprintf(
			"SELECT id FROM pull_requests WHERE repository_id = '%s' ORDER BY id", repoIDText,
		))
		if err != nil {
			t.Fatalf("query remaining pull requests: %v", err)
		}
		if len(remaining) <= 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(remaining) != 1 || remaining[0].ID != fmt.Sprintf("fresh-%d", suffix) {
		t.Fatalf("expected only the fresh row to survive after the mutation applied, got %+v", remaining)
	}
}

func openIntegrationClickHouse(t *testing.T, cfg config.Config) *clickhouse.DB {
	t.Helper()

	db, err := clickhouse.Open(cfg.ClickHouse, nil)
	if err != nil {
		t.Skipf("skip integration test: clickhouse unavailable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := clickhouse.EnsureSchema(ctx, db, cfg.DataLifecycle); err != nil {
		t.Skipf("skip integration test: clickhouse ensure schema failed: %v", err)
	}
	t.Cleanup(db.Close)

	return db
}

func formatUUID(value pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value.Bytes[0:4], value.Bytes[4:6], value.Bytes[6:8], value.Bytes[8:10], value.Bytes[10:16])
}

const clickHouseDateTimeLayout = "2006-01-02 15:04:05"

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
