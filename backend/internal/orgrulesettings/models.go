package orgrulesettings

import "time"

// Response reports the effective rule settings for an organization — the
// stored overrides merged over the boot-time defaults.
type Response struct {
	LargePR             LargePRSettings             `json:"largePR"`
	SlowReview          SlowReviewSettings          `json:"slowReview"`
	Hotspot             HotspotSettings             `json:"hotspot"`
	DeploymentFailure   DeploymentFailureSettings   `json:"deploymentFailure"`
	ReviewConcentration ReviewConcentrationSettings `json:"reviewConcentration"`
	Bottleneck          BottleneckSettings          `json:"bottleneck"`
	Metrics             MetricsSettings             `json:"metrics"`
	UpdatedAt           *time.Time                  `json:"updatedAt,omitempty"`
}

type LargePRSettings struct {
	Enabled               bool `json:"enabled"`
	FilesThreshold        int  `json:"filesThreshold"`
	TotalChangesThreshold int  `json:"totalChangesThreshold"`
}

type SlowReviewSettings struct {
	Enabled            bool    `json:"enabled"`
	WaitHoursThreshold float64 `json:"waitHoursThreshold"`
}

type HotspotSettings struct {
	Enabled        bool `json:"enabled"`
	ScoreThreshold int  `json:"scoreThreshold"`
}

type DeploymentFailureSettings struct {
	Enabled              bool    `json:"enabled"`
	MinimumDeployments   int     `json:"minimumDeployments"`
	FailureRateThreshold float64 `json:"failureRateThreshold"`
}

type ReviewConcentrationSettings struct {
	Enabled            bool    `json:"enabled"`
	MinimumReviewCount int     `json:"minimumReviewCount"`
	ShareThreshold     float64 `json:"shareThreshold"`
}

type BottleneckSettings struct {
	Enabled                    bool    `json:"enabled"`
	MinimumMergedCount         int     `json:"minimumMergedCount"`
	AverageCycleHoursThreshold float64 `json:"averageCycleHoursThreshold"`
	StaleOpenCountThreshold    int     `json:"staleOpenCountThreshold"`
	StaleOpenAgeDays           int     `json:"staleOpenAgeDays"`
}

type MetricsSettings struct {
	DefaultDayType         string  `json:"defaultDayType"`
	HotspotCommitWeight    float64 `json:"hotspotCommitWeight"`
	HotspotAdditionsWeight float64 `json:"hotspotAdditionsWeight"`
	HotspotDeletionsWeight float64 `json:"hotspotDeletionsWeight"`
}

// UpdateRequest carries a partial override — only non-nil fields change the
// stored configuration. Fields left nil keep their previous value (or the
// boot-time default if never overridden).
type UpdateRequest struct {
	LargePR             *LargePRUpdate             `json:"largePR"`
	SlowReview          *SlowReviewUpdate          `json:"slowReview"`
	Hotspot             *HotspotUpdate             `json:"hotspot"`
	DeploymentFailure   *DeploymentFailureUpdate   `json:"deploymentFailure"`
	ReviewConcentration *ReviewConcentrationUpdate `json:"reviewConcentration"`
	Bottleneck          *BottleneckUpdate          `json:"bottleneck"`
	Metrics             *MetricsUpdate             `json:"metrics"`
}

type LargePRUpdate struct {
	Enabled               *bool `json:"enabled"`
	FilesThreshold        *int  `json:"filesThreshold"`
	TotalChangesThreshold *int  `json:"totalChangesThreshold"`
}

type SlowReviewUpdate struct {
	Enabled            *bool    `json:"enabled"`
	WaitHoursThreshold *float64 `json:"waitHoursThreshold"`
}

type HotspotUpdate struct {
	Enabled        *bool `json:"enabled"`
	ScoreThreshold *int  `json:"scoreThreshold"`
}

type DeploymentFailureUpdate struct {
	Enabled              *bool    `json:"enabled"`
	MinimumDeployments   *int     `json:"minimumDeployments"`
	FailureRateThreshold *float64 `json:"failureRateThreshold"`
}

type ReviewConcentrationUpdate struct {
	Enabled            *bool    `json:"enabled"`
	MinimumReviewCount *int     `json:"minimumReviewCount"`
	ShareThreshold     *float64 `json:"shareThreshold"`
}

type BottleneckUpdate struct {
	Enabled                    *bool    `json:"enabled"`
	MinimumMergedCount         *int     `json:"minimumMergedCount"`
	AverageCycleHoursThreshold *float64 `json:"averageCycleHoursThreshold"`
	StaleOpenCountThreshold    *int     `json:"staleOpenCountThreshold"`
	StaleOpenAgeDays           *int     `json:"staleOpenAgeDays"`
}

type MetricsUpdate struct {
	DefaultDayType         *string  `json:"defaultDayType"`
	HotspotCommitWeight    *float64 `json:"hotspotCommitWeight"`
	HotspotAdditionsWeight *float64 `json:"hotspotAdditionsWeight"`
	HotspotDeletionsWeight *float64 `json:"hotspotDeletionsWeight"`
}

// storedConfig is the on-disk (JSONB) representation: only the fields an
// organization has explicitly overridden are present. Absent fields fall
// back to the boot-time defaults at resolve time.
type storedConfig struct {
	LargePR             *LargePRUpdate             `json:"largePR,omitempty"`
	SlowReview          *SlowReviewUpdate          `json:"slowReview,omitempty"`
	Hotspot             *HotspotUpdate             `json:"hotspot,omitempty"`
	DeploymentFailure   *DeploymentFailureUpdate   `json:"deploymentFailure,omitempty"`
	ReviewConcentration *ReviewConcentrationUpdate `json:"reviewConcentration,omitempty"`
	Bottleneck          *BottleneckUpdate          `json:"bottleneck,omitempty"`
	Metrics             *MetricsUpdate             `json:"metrics,omitempty"`
}
