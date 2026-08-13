package metricdefinition

import (
	"testing"

	"github.com/PangIkp/devlens/backend/internal/metrics"
)

func TestDecodeRepositoryMetricsRuleConfigUsesDefaultsWhenPayloadEmpty(t *testing.T) {
	t.Parallel()

	defaults := metrics.RuleConfig{
		DefaultDayType:         metrics.DayTypeCalendar,
		HotspotCommitWeight:    1,
		HotspotAdditionsWeight: 2,
		HotspotDeletionsWeight: 3,
	}

	cfg, err := decodeRepositoryMetricsRuleConfig(nil, defaults)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg != defaults {
		t.Fatalf("expected defaults to be preserved, got %+v", cfg)
	}
}

func TestDecodeRepositoryMetricsRuleConfigAppliesOverrides(t *testing.T) {
	t.Parallel()

	defaults := metrics.RuleConfig{
		DefaultDayType:         metrics.DayTypeCalendar,
		HotspotCommitWeight:    1,
		HotspotAdditionsWeight: 1,
		HotspotDeletionsWeight: 1,
	}

	cfg, err := decodeRepositoryMetricsRuleConfig([]byte(`{"defaultDayType":"business","hotspotCommitWeight":2,"hotspotDeletionsWeight":4}`), defaults)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.DefaultDayType != metrics.DayTypeBusiness {
		t.Fatalf("unexpected default day type %q", cfg.DefaultDayType)
	}
	if cfg.HotspotCommitWeight != 2 {
		t.Fatalf("unexpected hotspot commit weight %v", cfg.HotspotCommitWeight)
	}
	if cfg.HotspotAdditionsWeight != 1 {
		t.Fatalf("expected additions weight fallback, got %v", cfg.HotspotAdditionsWeight)
	}
	if cfg.HotspotDeletionsWeight != 4 {
		t.Fatalf("unexpected hotspot deletions weight %v", cfg.HotspotDeletionsWeight)
	}
}

func TestDecodeRepositoryMetricsRuleConfigRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	defaults := metrics.DefaultRuleConfig()

	cases := []struct {
		name    string
		payload string
	}{
		{name: "invalid day type", payload: `{"defaultDayType":"weird"}`},
		{name: "negative weight", payload: `{"hotspotCommitWeight":-1}`},
		{name: "all zero weights", payload: `{"hotspotCommitWeight":0,"hotspotAdditionsWeight":0,"hotspotDeletionsWeight":0}`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := decodeRepositoryMetricsRuleConfig([]byte(tc.payload), defaults); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
