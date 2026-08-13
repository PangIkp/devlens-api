package githubwebhook

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestRepositoryEnqueueWebhookSyncIsIdempotentForDuplicateDelivery(t *testing.T) {
	t.Parallel()

	repo, db := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	graph := seedWebhookIntegrationGraph(t, ctx, db)
	deliveryID := fmt.Sprintf("delivery-dup-%d", time.Now().UTC().UnixNano())
	action := "synchronize"
	payload := []byte(`{"action":"synchronize","repository":{"id":42}}`)

	first, err := repo.EnqueueWebhookSync(ctx, &graph.repositoryID, &graph.installationID, deliveryID, "push", &action, payload, true)
	if err != nil {
		t.Fatalf("enqueue first delivery: %v", err)
	}
	if first.duplicate {
		t.Fatalf("expected first delivery not duplicate: %+v", first)
	}
	if first.syncJobID == nil {
		t.Fatalf("expected sync job id on first delivery: %+v", first)
	}
	if first.processingStatus != "enqueued" {
		t.Fatalf("expected enqueued status, got %q", first.processingStatus)
	}

	second, err := repo.EnqueueWebhookSync(ctx, &graph.repositoryID, &graph.installationID, deliveryID, "push", &action, payload, true)
	if err != nil {
		t.Fatalf("enqueue duplicate delivery: %v", err)
	}
	if !second.duplicate {
		t.Fatalf("expected duplicate delivery result: %+v", second)
	}
	if second.syncJobID == nil || *second.syncJobID != *first.syncJobID {
		t.Fatalf("expected duplicate to return original sync job id, got first=%v second=%v", first.syncJobID, second.syncJobID)
	}
	if second.processingStatus != "enqueued" {
		t.Fatalf("expected duplicate processing status enqueued, got %q", second.processingStatus)
	}

	var syncJobCount int
	if err := db.Pool().QueryRow(ctx, `SELECT COUNT(*)::int FROM sync_jobs WHERE repository_id = $1`, parseUUID(graph.repositoryID)).Scan(&syncJobCount); err != nil {
		t.Fatalf("count sync jobs: %v", err)
	}
	if syncJobCount != 1 {
		t.Fatalf("expected exactly one sync job for duplicate delivery, got %d", syncJobCount)
	}
}

func TestRepositorySchedulesAndListsRetryableDeliveries(t *testing.T) {
	t.Parallel()

	repo, db := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	graph := seedWebhookIntegrationGraph(t, ctx, db)
	deliveryID := fmt.Sprintf("delivery-retry-%d", time.Now().UTC().UnixNano())
	payload := []byte(`{"action":"created","installation":{"id":42}}`)

	result, err := repo.EnqueueWebhookSync(ctx, &graph.repositoryID, &graph.installationID, deliveryID, "installation", nil, payload, false)
	if err != nil {
		t.Fatalf("enqueue delivery: %v", err)
	}
	if result.processingStatus != "ignored" {
		t.Fatalf("expected ignored processing status, got %q", result.processingStatus)
	}

	failedAt := time.Now().UTC()
	nextRetryAt := failedAt.Add(-time.Minute)
	if err := repo.ScheduleRetry(ctx, deliveryID, "temporary failure", 2, failedAt, nextRetryAt); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	ids, err := repo.ListRetryableDeliveryIDs(ctx, 10, time.Now().UTC())
	if err != nil {
		t.Fatalf("list retryable deliveries: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("expected retryable delivery ids")
	}

	found := false
	for _, id := range ids {
		if id == deliveryID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected delivery %q in retry list %#v", deliveryID, ids)
	}

	processedAt := time.Now().UTC()
	if err := repo.MarkDeliveryStatus(ctx, deliveryID, "processed", nil, &processedAt); err != nil {
		t.Fatalf("mark delivery processed: %v", err)
	}

	ids, err = repo.ListRetryableDeliveryIDs(ctx, 10, time.Now().UTC())
	if err != nil {
		t.Fatalf("list retryable deliveries after processed: %v", err)
	}
	for _, id := range ids {
		if id == deliveryID {
			t.Fatalf("did not expect processed delivery %q in retry list %#v", deliveryID, ids)
		}
	}
}

type integrationGraph struct {
	installationID int64
	repositoryID   string
}

func openIntegrationRepository(t *testing.T) (*Repository, *postgres.DB) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Postgres.ConnectTimeout)
	defer cancel()

	db, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Skipf("skip integration test: postgres unavailable: %v", err)
	}
	t.Cleanup(db.Close)

	return NewRepository(db), db
}

func seedWebhookIntegrationGraph(t *testing.T, ctx context.Context, db *postgres.DB) integrationGraph {
	t.Helper()

	orgID := newUUID()
	installationUUID := newUUID()
	repositoryUUID := newUUID()
	suffix := time.Now().UTC().UnixNano()
	installationID := suffix

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO organizations (id, github_id, name, slug, created_at)
		VALUES ($1, $2, $3, $4, NOW())`,
		orgID,
		suffix,
		"Webhook Integration Org",
		fmt.Sprintf("webhook-int-%d", suffix),
	); err != nil {
		t.Fatalf("insert organization: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO github_installations (
			id, organization_id, installation_id, installed_at, account_login, account_type, target_type, status, permissions_json, installed_by_github_user_id, updated_at
		) VALUES (
			$1, $2, $3, NOW(), 'pangikp', 'User', 'account', 'connected', '{}'::jsonb, $4, NOW()
		)`,
		installationUUID,
		orgID,
		installationID,
		suffix,
	); err != nil {
		t.Fatalf("insert github installation: %v", err)
	}

	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO repositories (
			id, organization_id, github_id, name, full_name, default_branch, is_active, github_installation_repository_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, TRUE, NULL
		)`,
		repositoryUUID,
		orgID,
		suffix,
		"devlens-api",
		fmt.Sprintf("pangikp/devlens-api-%d", suffix),
		"main",
	); err != nil {
		t.Fatalf("insert repository: %v", err)
	}

	return integrationGraph{
		installationID: installationID,
		repositoryID:   uuidString(repositoryUUID),
	}
}

func uuidString(value pgtype.UUID) string {
	return value.String()
}
