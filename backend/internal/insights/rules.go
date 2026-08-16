package insights

import "strings"

func DefaultRuleConfig() RuleConfig {
	return RuleConfig{
		LargePR: LargePRRuleConfig{
			Enabled:                     true,
			FilesThreshold:              25,
			TotalChangesThreshold:       800,
			HighSeverityFilesThreshold:  50,
			HighSeverityChangeThreshold: 1600,
		},
		SlowReview: SlowReviewRuleConfig{
			Enabled:                        true,
			WaitHoursThreshold:             24,
			HighSeverityWaitHoursThreshold: 72,
		},
		Hotspot: HotspotRuleConfig{
			Enabled:                    true,
			ScoreThreshold:             150,
			HighSeverityScoreThreshold: 300,
			TopFilesLimit:              10,
		},
		DeploymentFailure: DeploymentFailureRuleConfig{
			Enabled:                 true,
			MinimumDeployments:      3,
			FailureRateThreshold:    0.30,
			HighSeverityFailureRate: 0.50,
		},
		ReviewConcentration: ReviewConcentrationRuleConfig{
			Enabled:                    true,
			MinimumReviewCount:         5,
			ShareThreshold:             0.60,
			HighSeverityShareThreshold: 0.75,
		},
		Bottleneck: BottleneckRuleConfig{
			Enabled:                         true,
			MinimumMergedCount:              3,
			AverageCycleHoursThreshold:      72,
			HighSeverityCycleHoursThreshold: 120,
			StaleOpenCountThreshold:         3,
			HighSeverityStaleOpenThreshold:  5,
			StaleOpenAgeDays:                7,
		},
		AutoReopen: AutoReopenRuleConfig{
			OnReviewed:      true,
			OnDismissed:     true,
			MinimumSeverity: SeverityLow,
		},
		Deduplicate: DeduplicateRuleConfig{
			Enabled: true,
			Version: 1,
		},
	}
}

func normalizeRuleConfig(cfg RuleConfig) RuleConfig {
	defaults := DefaultRuleConfig()

	if cfg.LargePR.FilesThreshold <= 0 {
		cfg.LargePR.FilesThreshold = defaults.LargePR.FilesThreshold
	}
	if cfg.LargePR.TotalChangesThreshold <= 0 {
		cfg.LargePR.TotalChangesThreshold = defaults.LargePR.TotalChangesThreshold
	}
	if cfg.LargePR.HighSeverityFilesThreshold < cfg.LargePR.FilesThreshold {
		cfg.LargePR.HighSeverityFilesThreshold = defaults.LargePR.HighSeverityFilesThreshold
	}
	if cfg.LargePR.HighSeverityChangeThreshold < cfg.LargePR.TotalChangesThreshold {
		cfg.LargePR.HighSeverityChangeThreshold = defaults.LargePR.HighSeverityChangeThreshold
	}

	if cfg.SlowReview.WaitHoursThreshold <= 0 {
		cfg.SlowReview.WaitHoursThreshold = defaults.SlowReview.WaitHoursThreshold
	}
	if cfg.SlowReview.HighSeverityWaitHoursThreshold < cfg.SlowReview.WaitHoursThreshold {
		cfg.SlowReview.HighSeverityWaitHoursThreshold = defaults.SlowReview.HighSeverityWaitHoursThreshold
	}

	if cfg.Hotspot.ScoreThreshold <= 0 {
		cfg.Hotspot.ScoreThreshold = defaults.Hotspot.ScoreThreshold
	}
	if cfg.Hotspot.HighSeverityScoreThreshold < cfg.Hotspot.ScoreThreshold {
		cfg.Hotspot.HighSeverityScoreThreshold = defaults.Hotspot.HighSeverityScoreThreshold
	}
	if cfg.Hotspot.TopFilesLimit <= 0 {
		cfg.Hotspot.TopFilesLimit = defaults.Hotspot.TopFilesLimit
	}

	if cfg.DeploymentFailure.MinimumDeployments <= 0 {
		cfg.DeploymentFailure.MinimumDeployments = defaults.DeploymentFailure.MinimumDeployments
	}
	if cfg.DeploymentFailure.FailureRateThreshold <= 0 || cfg.DeploymentFailure.FailureRateThreshold > 1 {
		cfg.DeploymentFailure.FailureRateThreshold = defaults.DeploymentFailure.FailureRateThreshold
	}
	if cfg.DeploymentFailure.HighSeverityFailureRate < cfg.DeploymentFailure.FailureRateThreshold || cfg.DeploymentFailure.HighSeverityFailureRate > 1 {
		cfg.DeploymentFailure.HighSeverityFailureRate = defaults.DeploymentFailure.HighSeverityFailureRate
	}

	if cfg.ReviewConcentration.MinimumReviewCount <= 0 {
		cfg.ReviewConcentration.MinimumReviewCount = defaults.ReviewConcentration.MinimumReviewCount
	}
	if cfg.ReviewConcentration.ShareThreshold <= 0 || cfg.ReviewConcentration.ShareThreshold > 1 {
		cfg.ReviewConcentration.ShareThreshold = defaults.ReviewConcentration.ShareThreshold
	}
	if cfg.ReviewConcentration.HighSeverityShareThreshold < cfg.ReviewConcentration.ShareThreshold || cfg.ReviewConcentration.HighSeverityShareThreshold > 1 {
		cfg.ReviewConcentration.HighSeverityShareThreshold = defaults.ReviewConcentration.HighSeverityShareThreshold
	}

	if cfg.Bottleneck.MinimumMergedCount <= 0 {
		cfg.Bottleneck.MinimumMergedCount = defaults.Bottleneck.MinimumMergedCount
	}
	if cfg.Bottleneck.AverageCycleHoursThreshold <= 0 {
		cfg.Bottleneck.AverageCycleHoursThreshold = defaults.Bottleneck.AverageCycleHoursThreshold
	}
	if cfg.Bottleneck.HighSeverityCycleHoursThreshold < cfg.Bottleneck.AverageCycleHoursThreshold {
		cfg.Bottleneck.HighSeverityCycleHoursThreshold = defaults.Bottleneck.HighSeverityCycleHoursThreshold
	}
	if cfg.Bottleneck.StaleOpenCountThreshold <= 0 {
		cfg.Bottleneck.StaleOpenCountThreshold = defaults.Bottleneck.StaleOpenCountThreshold
	}
	if cfg.Bottleneck.HighSeverityStaleOpenThreshold < cfg.Bottleneck.StaleOpenCountThreshold {
		cfg.Bottleneck.HighSeverityStaleOpenThreshold = defaults.Bottleneck.HighSeverityStaleOpenThreshold
	}
	if cfg.Bottleneck.StaleOpenAgeDays <= 0 {
		cfg.Bottleneck.StaleOpenAgeDays = defaults.Bottleneck.StaleOpenAgeDays
	}

	cfg.AutoReopen.MinimumSeverity = normalizeSeverity(cfg.AutoReopen.MinimumSeverity, defaults.AutoReopen.MinimumSeverity)
	if cfg.Deduplicate.Version <= 0 {
		cfg.Deduplicate.Version = defaults.Deduplicate.Version
	}

	return cfg
}

func normalizeSeverity(value string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SeverityLow:
		return SeverityLow
	case SeverityMedium:
		return SeverityMedium
	case SeverityHigh:
		return SeverityHigh
	case SeverityCritical:
		return SeverityCritical
	default:
		return fallback
	}
}
