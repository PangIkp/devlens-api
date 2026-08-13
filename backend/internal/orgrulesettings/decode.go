package orgrulesettings

import (
	"encoding/json"
	"fmt"

	"github.com/PangIkp/devlens/backend/internal/insights"
	"github.com/PangIkp/devlens/backend/internal/metrics"
)

func decodeStoredConfig(raw []byte) (storedConfig, error) {
	var cfg storedConfig
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return storedConfig{}, fmt.Errorf("decode organization rule settings: %w", err)
	}
	return cfg, nil
}

// mergeStoredConfig overlays a partial update onto the previously stored
// config. Fields present in req replace the corresponding stored field
// wholesale; fields absent from req keep whatever was stored before.
func mergeStoredConfig(stored storedConfig, req UpdateRequest) storedConfig {
	merged := stored
	if req.LargePR != nil {
		merged.LargePR = req.LargePR
	}
	if req.SlowReview != nil {
		merged.SlowReview = req.SlowReview
	}
	if req.Hotspot != nil {
		merged.Hotspot = req.Hotspot
	}
	if req.DeploymentFailure != nil {
		merged.DeploymentFailure = req.DeploymentFailure
	}
	if req.ReviewConcentration != nil {
		merged.ReviewConcentration = req.ReviewConcentration
	}
	if req.Bottleneck != nil {
		merged.Bottleneck = req.Bottleneck
	}
	if req.Metrics != nil {
		merged.Metrics = req.Metrics
	}
	return merged
}

func applyInsightOverrides(stored storedConfig, defaults insights.RuleConfig) insights.RuleConfig {
	cfg := defaults

	if o := stored.LargePR; o != nil {
		if o.Enabled != nil {
			cfg.LargePR.Enabled = *o.Enabled
		}
		if o.FilesThreshold != nil {
			cfg.LargePR.FilesThreshold = *o.FilesThreshold
		}
		if o.TotalChangesThreshold != nil {
			cfg.LargePR.TotalChangesThreshold = *o.TotalChangesThreshold
		}
	}
	if o := stored.SlowReview; o != nil {
		if o.Enabled != nil {
			cfg.SlowReview.Enabled = *o.Enabled
		}
		if o.WaitHoursThreshold != nil {
			cfg.SlowReview.WaitHoursThreshold = *o.WaitHoursThreshold
		}
	}
	if o := stored.Hotspot; o != nil {
		if o.Enabled != nil {
			cfg.Hotspot.Enabled = *o.Enabled
		}
		if o.ScoreThreshold != nil {
			cfg.Hotspot.ScoreThreshold = *o.ScoreThreshold
		}
	}
	if o := stored.DeploymentFailure; o != nil {
		if o.Enabled != nil {
			cfg.DeploymentFailure.Enabled = *o.Enabled
		}
		if o.MinimumDeployments != nil {
			cfg.DeploymentFailure.MinimumDeployments = *o.MinimumDeployments
		}
		if o.FailureRateThreshold != nil {
			cfg.DeploymentFailure.FailureRateThreshold = *o.FailureRateThreshold
		}
	}
	if o := stored.ReviewConcentration; o != nil {
		if o.Enabled != nil {
			cfg.ReviewConcentration.Enabled = *o.Enabled
		}
		if o.MinimumReviewCount != nil {
			cfg.ReviewConcentration.MinimumReviewCount = *o.MinimumReviewCount
		}
		if o.ShareThreshold != nil {
			cfg.ReviewConcentration.ShareThreshold = *o.ShareThreshold
		}
	}
	if o := stored.Bottleneck; o != nil {
		if o.Enabled != nil {
			cfg.Bottleneck.Enabled = *o.Enabled
		}
		if o.MinimumMergedCount != nil {
			cfg.Bottleneck.MinimumMergedCount = *o.MinimumMergedCount
		}
		if o.AverageCycleHoursThreshold != nil {
			cfg.Bottleneck.AverageCycleHoursThreshold = *o.AverageCycleHoursThreshold
		}
		if o.StaleOpenCountThreshold != nil {
			cfg.Bottleneck.StaleOpenCountThreshold = *o.StaleOpenCountThreshold
		}
		if o.StaleOpenAgeDays != nil {
			cfg.Bottleneck.StaleOpenAgeDays = *o.StaleOpenAgeDays
		}
	}

	return cfg
}

func applyMetricsOverrides(stored storedConfig, defaults metrics.RuleConfig) metrics.RuleConfig {
	cfg := defaults
	o := stored.Metrics
	if o == nil {
		return cfg
	}
	if o.DefaultDayType != nil {
		cfg.DefaultDayType = *o.DefaultDayType
	}
	if o.HotspotCommitWeight != nil {
		cfg.HotspotCommitWeight = *o.HotspotCommitWeight
	}
	if o.HotspotAdditionsWeight != nil {
		cfg.HotspotAdditionsWeight = *o.HotspotAdditionsWeight
	}
	if o.HotspotDeletionsWeight != nil {
		cfg.HotspotDeletionsWeight = *o.HotspotDeletionsWeight
	}
	return cfg
}

func buildResponse(stored storedConfig, insightDefaults insights.RuleConfig, metricsDefaults metrics.RuleConfig) Response {
	resolvedInsights := applyInsightOverrides(stored, insightDefaults)
	resolvedMetrics := applyMetricsOverrides(stored, metricsDefaults)

	return Response{
		LargePR: LargePRSettings{
			Enabled:               resolvedInsights.LargePR.Enabled,
			FilesThreshold:        resolvedInsights.LargePR.FilesThreshold,
			TotalChangesThreshold: resolvedInsights.LargePR.TotalChangesThreshold,
		},
		SlowReview: SlowReviewSettings{
			Enabled:            resolvedInsights.SlowReview.Enabled,
			WaitHoursThreshold: resolvedInsights.SlowReview.WaitHoursThreshold,
		},
		Hotspot: HotspotSettings{
			Enabled:        resolvedInsights.Hotspot.Enabled,
			ScoreThreshold: resolvedInsights.Hotspot.ScoreThreshold,
		},
		DeploymentFailure: DeploymentFailureSettings{
			Enabled:              resolvedInsights.DeploymentFailure.Enabled,
			MinimumDeployments:   resolvedInsights.DeploymentFailure.MinimumDeployments,
			FailureRateThreshold: resolvedInsights.DeploymentFailure.FailureRateThreshold,
		},
		ReviewConcentration: ReviewConcentrationSettings{
			Enabled:            resolvedInsights.ReviewConcentration.Enabled,
			MinimumReviewCount: resolvedInsights.ReviewConcentration.MinimumReviewCount,
			ShareThreshold:     resolvedInsights.ReviewConcentration.ShareThreshold,
		},
		Bottleneck: BottleneckSettings{
			Enabled:                    resolvedInsights.Bottleneck.Enabled,
			MinimumMergedCount:         resolvedInsights.Bottleneck.MinimumMergedCount,
			AverageCycleHoursThreshold: resolvedInsights.Bottleneck.AverageCycleHoursThreshold,
			StaleOpenCountThreshold:    resolvedInsights.Bottleneck.StaleOpenCountThreshold,
			StaleOpenAgeDays:           resolvedInsights.Bottleneck.StaleOpenAgeDays,
		},
		Metrics: MetricsSettings{
			DefaultDayType:         resolvedMetrics.DefaultDayType,
			HotspotCommitWeight:    resolvedMetrics.HotspotCommitWeight,
			HotspotAdditionsWeight: resolvedMetrics.HotspotAdditionsWeight,
			HotspotDeletionsWeight: resolvedMetrics.HotspotDeletionsWeight,
		},
	}
}

func validateStoredConfig(stored storedConfig, insightDefaults insights.RuleConfig, metricsDefaults metrics.RuleConfig) error {
	var issues []ValidationIssue

	if o := stored.LargePR; o != nil {
		if o.FilesThreshold != nil && *o.FilesThreshold <= 0 {
			issues = append(issues, ValidationIssue{Field: "largePR.filesThreshold", Message: "must be greater than 0"})
		}
		if o.TotalChangesThreshold != nil && *o.TotalChangesThreshold <= 0 {
			issues = append(issues, ValidationIssue{Field: "largePR.totalChangesThreshold", Message: "must be greater than 0"})
		}
	}
	if o := stored.SlowReview; o != nil {
		if o.WaitHoursThreshold != nil && *o.WaitHoursThreshold <= 0 {
			issues = append(issues, ValidationIssue{Field: "slowReview.waitHoursThreshold", Message: "must be greater than 0"})
		}
	}
	if o := stored.Hotspot; o != nil {
		if o.ScoreThreshold != nil && *o.ScoreThreshold <= 0 {
			issues = append(issues, ValidationIssue{Field: "hotspot.scoreThreshold", Message: "must be greater than 0"})
		}
	}
	if o := stored.DeploymentFailure; o != nil {
		if o.MinimumDeployments != nil && *o.MinimumDeployments <= 0 {
			issues = append(issues, ValidationIssue{Field: "deploymentFailure.minimumDeployments", Message: "must be greater than 0"})
		}
		if o.FailureRateThreshold != nil && (*o.FailureRateThreshold <= 0 || *o.FailureRateThreshold > 1) {
			issues = append(issues, ValidationIssue{Field: "deploymentFailure.failureRateThreshold", Message: "must be greater than 0 and less than or equal to 1"})
		}
	}
	if o := stored.ReviewConcentration; o != nil {
		if o.MinimumReviewCount != nil && *o.MinimumReviewCount <= 0 {
			issues = append(issues, ValidationIssue{Field: "reviewConcentration.minimumReviewCount", Message: "must be greater than 0"})
		}
		if o.ShareThreshold != nil && (*o.ShareThreshold <= 0 || *o.ShareThreshold > 1) {
			issues = append(issues, ValidationIssue{Field: "reviewConcentration.shareThreshold", Message: "must be greater than 0 and less than or equal to 1"})
		}
	}
	if o := stored.Bottleneck; o != nil {
		if o.MinimumMergedCount != nil && *o.MinimumMergedCount <= 0 {
			issues = append(issues, ValidationIssue{Field: "bottleneck.minimumMergedCount", Message: "must be greater than 0"})
		}
		if o.AverageCycleHoursThreshold != nil && *o.AverageCycleHoursThreshold <= 0 {
			issues = append(issues, ValidationIssue{Field: "bottleneck.averageCycleHoursThreshold", Message: "must be greater than 0"})
		}
		if o.StaleOpenCountThreshold != nil && *o.StaleOpenCountThreshold <= 0 {
			issues = append(issues, ValidationIssue{Field: "bottleneck.staleOpenCountThreshold", Message: "must be greater than 0"})
		}
		if o.StaleOpenAgeDays != nil && *o.StaleOpenAgeDays <= 0 {
			issues = append(issues, ValidationIssue{Field: "bottleneck.staleOpenAgeDays", Message: "must be greater than 0"})
		}
	}
	if o := stored.Metrics; o != nil {
		if o.DefaultDayType != nil && *o.DefaultDayType != metrics.DayTypeCalendar && *o.DefaultDayType != metrics.DayTypeBusiness {
			issues = append(issues, ValidationIssue{Field: "metrics.defaultDayType", Message: fmt.Sprintf("must be one of %s, %s", metrics.DayTypeCalendar, metrics.DayTypeBusiness)})
		}
		if o.HotspotCommitWeight != nil && *o.HotspotCommitWeight < 0 {
			issues = append(issues, ValidationIssue{Field: "metrics.hotspotCommitWeight", Message: "must be greater than or equal to 0"})
		}
		if o.HotspotAdditionsWeight != nil && *o.HotspotAdditionsWeight < 0 {
			issues = append(issues, ValidationIssue{Field: "metrics.hotspotAdditionsWeight", Message: "must be greater than or equal to 0"})
		}
		if o.HotspotDeletionsWeight != nil && *o.HotspotDeletionsWeight < 0 {
			issues = append(issues, ValidationIssue{Field: "metrics.hotspotDeletionsWeight", Message: "must be greater than or equal to 0"})
		}
		resolved := applyMetricsOverrides(stored, metricsDefaults)
		if resolved.HotspotCommitWeight == 0 && resolved.HotspotAdditionsWeight == 0 && resolved.HotspotDeletionsWeight == 0 {
			issues = append(issues, ValidationIssue{Field: "metrics", Message: "must have at least one hotspot weight greater than 0"})
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}
