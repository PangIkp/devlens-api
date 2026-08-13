package orgrulesettings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/insights"
	"github.com/PangIkp/devlens/backend/internal/metrics"
)

type stubStore struct {
	ensureFn func(context.Context, string) error
	getFn    func(context.Context, string) ([]byte, *time.Time, error)
	upsertFn func(context.Context, string, []byte, *string) (*time.Time, error)
}

func (s stubStore) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	if s.ensureFn != nil {
		return s.ensureFn(ctx, organizationID)
	}
	return nil
}
func (s stubStore) Get(ctx context.Context, organizationID string) ([]byte, *time.Time, error) {
	if s.getFn != nil {
		return s.getFn(ctx, organizationID)
	}
	return nil, nil, nil
}
func (s stubStore) Upsert(ctx context.Context, organizationID string, configJSON []byte, updatedBy *string) (*time.Time, error) {
	if s.upsertFn != nil {
		return s.upsertFn(ctx, organizationID, configJSON, updatedBy)
	}
	return nil, nil
}

func testDefaults() (insights.RuleConfig, metrics.RuleConfig) {
	insightDefaults := insights.DefaultRuleConfig()
	metricsDefaults := metrics.DefaultRuleConfig()
	return insightDefaults, metricsDefaults
}

func TestGetReturnsBootDefaultsWhenNoOverrideStored(t *testing.T) {
	t.Parallel()

	insightDefaults, metricsDefaults := testDefaults()
	service := NewService(stubStore{
		getFn: func(context.Context, string) ([]byte, *time.Time, error) { return nil, nil, nil },
	}, insightDefaults, metricsDefaults)

	response, err := service.Get(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if response.LargePR.FilesThreshold != insightDefaults.LargePR.FilesThreshold {
		t.Fatalf("expected default files threshold %d, got %d", insightDefaults.LargePR.FilesThreshold, response.LargePR.FilesThreshold)
	}
	if !response.LargePR.Enabled {
		t.Fatal("expected large PR rule enabled by default")
	}
	if response.Metrics.DefaultDayType != metricsDefaults.DefaultDayType {
		t.Fatalf("expected default day type %q, got %q", metricsDefaults.DefaultDayType, response.Metrics.DefaultDayType)
	}
}

func TestUpdateMergesPartialOverrideOntoExistingStoredConfig(t *testing.T) {
	t.Parallel()

	insightDefaults, metricsDefaults := testDefaults()
	existing, err := json.Marshal(storedConfig{
		LargePR: &LargePRUpdate{FilesThreshold: intPtr(40)},
	})
	if err != nil {
		t.Fatalf("marshal existing: %v", err)
	}

	var upserted []byte
	service := NewService(stubStore{
		getFn: func(context.Context, string) ([]byte, *time.Time, error) { return existing, nil, nil },
		upsertFn: func(_ context.Context, _ string, configJSON []byte, _ *string) (*time.Time, error) {
			upserted = configJSON
			return nil, nil
		},
	}, insightDefaults, metricsDefaults)

	// Update only slowReview; largePR override from the existing config must survive.
	response, err := service.Update(context.Background(), "org-1", UpdateRequest{
		SlowReview: &SlowReviewUpdate{WaitHoursThreshold: floatPtr(12)},
	}, nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if response.LargePR.FilesThreshold != 40 {
		t.Fatalf("expected largePR override to survive merge, got %d", response.LargePR.FilesThreshold)
	}
	if response.SlowReview.WaitHoursThreshold != 12 {
		t.Fatalf("expected slowReview override applied, got %v", response.SlowReview.WaitHoursThreshold)
	}

	var persisted storedConfig
	if err := json.Unmarshal(upserted, &persisted); err != nil {
		t.Fatalf("decode persisted config: %v", err)
	}
	if persisted.LargePR == nil || persisted.LargePR.FilesThreshold == nil || *persisted.LargePR.FilesThreshold != 40 {
		t.Fatalf("expected persisted config to retain largePR override, got %+v", persisted.LargePR)
	}
	if persisted.SlowReview == nil || persisted.SlowReview.WaitHoursThreshold == nil || *persisted.SlowReview.WaitHoursThreshold != 12 {
		t.Fatalf("expected persisted config to include new slowReview override, got %+v", persisted.SlowReview)
	}
}

func TestUpdateRejectsInvalidThreshold(t *testing.T) {
	t.Parallel()

	insightDefaults, metricsDefaults := testDefaults()
	service := NewService(stubStore{
		getFn: func(context.Context, string) ([]byte, *time.Time, error) { return nil, nil, nil },
	}, insightDefaults, metricsDefaults)

	_, err := service.Update(context.Background(), "org-1", UpdateRequest{
		LargePR: &LargePRUpdate{FilesThreshold: intPtr(0)},
	}, nil)

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestResolveInsightRulesAppliesEnabledOverride(t *testing.T) {
	t.Parallel()

	insightDefaults, metricsDefaults := testDefaults()
	stored, err := json.Marshal(storedConfig{
		Hotspot: &HotspotUpdate{Enabled: boolPtr(false)},
	})
	if err != nil {
		t.Fatalf("marshal stored: %v", err)
	}

	service := NewService(stubStore{
		getFn: func(context.Context, string) ([]byte, *time.Time, error) { return stored, nil, nil },
	}, insightDefaults, metricsDefaults)

	rules, err := service.ResolveInsightRules(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("resolve insight rules: %v", err)
	}
	if rules.Hotspot.Enabled {
		t.Fatal("expected hotspot rule to be disabled by override")
	}
	if !rules.LargePR.Enabled {
		t.Fatal("expected largePR rule to remain enabled (untouched by override)")
	}
}

func TestResolveMetricsRulesFallsBackToDefaultsOnStoreError(t *testing.T) {
	t.Parallel()

	insightDefaults, metricsDefaults := testDefaults()
	service := NewService(stubStore{
		getFn: func(context.Context, string) ([]byte, *time.Time, error) { return nil, nil, errors.New("boom") },
	}, insightDefaults, metricsDefaults)

	rules, err := service.ResolveMetricsRules(context.Background(), "org-1")
	if err == nil {
		t.Fatal("expected error to be surfaced")
	}
	if rules != metricsDefaults {
		t.Fatalf("expected fallback to defaults on error, got %+v", rules)
	}
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
func boolPtr(v bool) *bool        { return &v }
