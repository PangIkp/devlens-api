package organizationmember

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

func TestDeleteWithOwnerGuardIntegration(t *testing.T) {
	t.Parallel()

	repo, db := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().UnixNano()
	orgID := newIntegrationUUID()
	userA := newIntegrationUUID()
	userB := newIntegrationUUID()
	memberA := newIntegrationUUID()
	memberB := newIntegrationUUID()

	mustIntegrationExec(t, ctx, db, `INSERT INTO organizations (id, github_id, name, slug, created_at, deleted_at) VALUES ($1, $2, $3, $4, NOW(), NULL)`,
		orgID, suffix, "Integration Member Org", fmt.Sprintf("integration-member-org-%d", suffix))
	mustIntegrationExec(t, ctx, db, `INSERT INTO users (id, email, name, created_at) VALUES ($1, $2, $3, NOW())`,
		userA, fmt.Sprintf("integration-member-a-%d@example.com", suffix), "Owner A")
	mustIntegrationExec(t, ctx, db, `INSERT INTO users (id, email, name, created_at) VALUES ($1, $2, $3, NOW())`,
		userB, fmt.Sprintf("integration-member-b-%d@example.com", suffix), "Owner B")
	mustIntegrationExec(t, ctx, db, `INSERT INTO organization_members (id, organization_id, user_id, role) VALUES ($1, $2, $3, 'owner')`,
		memberA, orgID, userA)
	mustIntegrationExec(t, ctx, db, `INSERT INTO organization_members (id, organization_id, user_id, role) VALUES ($1, $2, $3, 'owner')`,
		memberB, orgID, userB)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = db.Pool().Exec(cleanupCtx, `DELETE FROM users WHERE id = ANY($1)`, []pgtype.UUID{userA, userB})
	})

	orgIDStr := formatIntegrationUUID(orgID)
	memberAStr := formatIntegrationUUID(memberA)
	memberBStr := formatIntegrationUUID(memberB)

	// Demoting one of two owners is fine.
	if _, err := repo.UpdateRoleWithOwnerGuard(ctx, UpdateParams{ID: memberAStr, Role: "member"}, orgIDStr, "owner"); err != nil {
		t.Fatalf("expected demotion of one of two owners to succeed, got %v", err)
	}

	// Now only memberB is an owner: demoting them must be rejected.
	if _, err := repo.UpdateRoleWithOwnerGuard(ctx, UpdateParams{ID: memberBStr, Role: "member"}, orgIDStr, "owner"); !errors.Is(err, ErrLastOwnerConflict) {
		t.Fatalf("expected ErrLastOwnerConflict demoting the last owner, got %v", err)
	}

	// Removing the last owner outright must also be rejected.
	if err := repo.DeleteWithOwnerGuard(ctx, orgIDStr, memberBStr, "owner"); !errors.Is(err, ErrLastOwnerConflict) {
		t.Fatalf("expected ErrLastOwnerConflict deleting the last owner, got %v", err)
	}

	// Deleting a non-owner member is unaffected by the guard.
	if err := repo.DeleteWithOwnerGuard(ctx, orgIDStr, memberAStr, "member"); err != nil {
		t.Fatalf("expected deleting a non-owner member to succeed, got %v", err)
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

func formatIntegrationUUID(value pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", value.Bytes[0:4], value.Bytes[4:6], value.Bytes[6:8], value.Bytes[8:10], value.Bytes[10:16])
}
