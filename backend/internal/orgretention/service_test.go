package orgretention

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubStore struct {
	ensureFn func(context.Context, string) error
	getFn    func(context.Context, string) (*int, *time.Time, error)
	upsertFn func(context.Context, string, *int, *string) (*time.Time, error)
}

func (s stubStore) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	if s.ensureFn != nil {
		return s.ensureFn(ctx, organizationID)
	}
	return nil
}
func (s stubStore) Get(ctx context.Context, organizationID string) (*int, *time.Time, error) {
	if s.getFn != nil {
		return s.getFn(ctx, organizationID)
	}
	return nil, nil, nil
}
func (s stubStore) Upsert(ctx context.Context, organizationID string, days *int, updatedBy *string) (*time.Time, error) {
	if s.upsertFn != nil {
		return s.upsertFn(ctx, organizationID, days, updatedBy)
	}
	return nil, nil
}

func TestGetReturnsGlobalDefaultWhenNoOverrideStored(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		getFn: func(context.Context, string) (*int, *time.Time, error) { return nil, nil, nil },
	}, 180)

	response, err := service.Get(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if response.AnalyticsRawRetentionDays != 180 {
		t.Fatalf("expected global default 180, got %d", response.AnalyticsRawRetentionDays)
	}
	if response.Enforced {
		t.Fatal("expected enforced to be false until per-org ClickHouse enforcement is built")
	}
}

func TestGetReturnsStoredOverride(t *testing.T) {
	t.Parallel()

	days := 30
	service := NewService(stubStore{
		getFn: func(context.Context, string) (*int, *time.Time, error) { return &days, nil, nil },
	}, 180)

	response, err := service.Get(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if response.AnalyticsRawRetentionDays != 30 {
		t.Fatalf("expected stored override 30, got %d", response.AnalyticsRawRetentionDays)
	}
}

func TestUpdateRejectsNonPositiveDays(t *testing.T) {
	t.Parallel()

	invalid := 0
	service := NewService(stubStore{}, 180)

	_, err := service.Update(context.Background(), "org-1", UpdateRequest{AnalyticsRawRetentionDays: &invalid}, nil)

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestUpdatePersistsOverride(t *testing.T) {
	t.Parallel()

	var upsertedDays *int
	service := NewService(stubStore{
		upsertFn: func(_ context.Context, _ string, days *int, _ *string) (*time.Time, error) {
			upsertedDays = days
			return nil, nil
		},
	}, 180)

	newDays := 45
	response, err := service.Update(context.Background(), "org-1", UpdateRequest{AnalyticsRawRetentionDays: &newDays}, nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if response.AnalyticsRawRetentionDays != 45 {
		t.Fatalf("expected 45, got %d", response.AnalyticsRawRetentionDays)
	}
	if upsertedDays == nil || *upsertedDays != 45 {
		t.Fatalf("expected store to persist 45, got %v", upsertedDays)
	}
}

func TestGetReturnsNotFoundForMissingOrganization(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		ensureFn: func(context.Context, string) error { return ErrOrganizationNotFound },
	}, 180)

	_, err := service.Get(context.Background(), "missing-org")
	if !errors.Is(err, ErrOrganizationNotFound) {
		t.Fatalf("expected ErrOrganizationNotFound, got %v", err)
	}
}
