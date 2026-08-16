package metricdefinition

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/metrics"
)

const MetricKeyRepositoryMetrics = "repository_metrics"

type repositoryMetricsConfig struct {
	DefaultDayType         *string  `json:"defaultDayType"`
	HotspotCommitWeight    *float64 `json:"hotspotCommitWeight"`
	HotspotAdditionsWeight *float64 `json:"hotspotAdditionsWeight"`
	HotspotDeletionsWeight *float64 `json:"hotspotDeletionsWeight"`
}

func decodeRepositoryMetricsRuleConfig(payload []byte, defaults metrics.RuleConfig) (metrics.RuleConfig, error) {
	if len(payload) == 0 {
		return defaults, nil
	}

	var raw repositoryMetricsConfig
	if err := json.Unmarshal(payload, &raw); err != nil {
		return metrics.RuleConfig{}, fmt.Errorf("decode repository metrics config: %w", err)
	}

	cfg := defaults
	if raw.DefaultDayType != nil {
		cfg.DefaultDayType = strings.TrimSpace(strings.ToLower(*raw.DefaultDayType))
	}
	if raw.HotspotCommitWeight != nil {
		cfg.HotspotCommitWeight = *raw.HotspotCommitWeight
	}
	if raw.HotspotAdditionsWeight != nil {
		cfg.HotspotAdditionsWeight = *raw.HotspotAdditionsWeight
	}
	if raw.HotspotDeletionsWeight != nil {
		cfg.HotspotDeletionsWeight = *raw.HotspotDeletionsWeight
	}

	if cfg.DefaultDayType != metrics.DayTypeBusiness && cfg.DefaultDayType != metrics.DayTypeCalendar {
		return metrics.RuleConfig{}, fmt.Errorf("repository metrics default day type must be one of %s, %s", metrics.DayTypeCalendar, metrics.DayTypeBusiness)
	}
	if cfg.HotspotCommitWeight < 0 {
		return metrics.RuleConfig{}, fmt.Errorf("repository metrics hotspot commit weight must be greater than or equal to 0")
	}
	if cfg.HotspotAdditionsWeight < 0 {
		return metrics.RuleConfig{}, fmt.Errorf("repository metrics hotspot additions weight must be greater than or equal to 0")
	}
	if cfg.HotspotDeletionsWeight < 0 {
		return metrics.RuleConfig{}, fmt.Errorf("repository metrics hotspot deletions weight must be greater than or equal to 0")
	}
	if cfg.HotspotCommitWeight == 0 && cfg.HotspotAdditionsWeight == 0 && cfg.HotspotDeletionsWeight == 0 {
		return metrics.RuleConfig{}, fmt.Errorf("repository metrics must have at least one hotspot weight greater than 0")
	}

	return cfg, nil
}
