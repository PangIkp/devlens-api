package organization

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres"
)

func TestRepositoryUpdateAndSoftDeleteIntegration(t *testing.T) {
	t.Parallel()

	repo := openIntegrationRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UTC().UnixNano()
	created, err := repo.Create(ctx, CreateParams{
		GithubID: suffix,
		Slug:     fmt.Sprintf("org-%d", suffix),
		Name:     "Integration Org",
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	updated, err := repo.Update(ctx, UpdateParams{
		ID:   created.ID,
		Slug: fmt.Sprintf("org-%d-updated", suffix),
		Name: "Integration Org Updated",
	})
	if err != nil {
		t.Fatalf("update organization: %v", err)
	}

	if updated.Slug != fmt.Sprintf("org-%d-updated", suffix) {
		t.Fatalf("expected updated slug, got %q", updated.Slug)
	}

	if updated.UpdatedAt == nil {
		t.Fatal("expected updatedAt to be set")
	}

	if err := repo.SoftDelete(ctx, created.ID); err != nil {
		t.Fatalf("soft delete organization: %v", err)
	}

	_, err = repo.GetByID(ctx, created.ID)
	if !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("expected organization not found after soft delete, got %v", err)
	}
}

func openIntegrationRepository(t *testing.T) *Repository {
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

	return NewRepository(db)
}
