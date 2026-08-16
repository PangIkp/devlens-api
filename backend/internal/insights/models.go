package insights

import "time"

const (
	TypeBottleneckDetection    = "bottleneck_detection"
	TypeLargePRDetection       = "large_pr_detection"
	TypeSlowReviewDetection    = "slow_review_detection"
	TypeHotspotDetection       = "hotspot_detection"
	TypeDeploymentFailureTrend = "deployment_failure_trend"
	TypeReviewConcentration    = "review_concentration"
	StatusOpen                 = "open"
	StatusReviewed             = "reviewed"
	StatusDismissed            = "dismissed"
	SeverityLow                = "low"
	SeverityMedium             = "medium"
	SeverityHigh               = "high"
	SeverityCritical           = "critical"
)

type ListParams struct {
	OrganizationID string
	RepositoryID   string
	Type           string
	Status         string
	From           time.Time
	To             time.Time
	Page           int
	PageSize       int
}

type ReviewRequest struct {
	ReviewedBy *string `json:"reviewedBy"`
}

type Insight struct {
	InsightKey     string         `json:"insightKey"`
	InsightType    string         `json:"insightType"`
	Status         string         `json:"status"`
	Severity       string         `json:"severity"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	OrganizationID string         `json:"organizationId"`
	RepositoryID   string         `json:"repositoryId,omitempty"`
	RepositoryName string         `json:"repositoryName,omitempty"`
	DetectedAt     time.Time      `json:"detectedAt"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	ReviewedBy     *string        `json:"reviewedBy,omitempty"`
	ReviewedAt     *time.Time     `json:"reviewedAt,omitempty"`
	DismissedAt    *time.Time     `json:"dismissedAt,omitempty"`
	ReopenedAt     *time.Time     `json:"reopenedAt,omitempty"`
}

type ListResult struct {
	Items      []Insight
	TotalItems int
}

type StatusResult struct {
	InsightKey  string     `json:"insightKey"`
	InsightType string     `json:"insightType"`
	Status      string     `json:"status"`
	ReviewedBy  *string    `json:"reviewedBy,omitempty"`
	ReviewedAt  *time.Time `json:"reviewedAt,omitempty"`
	DismissedAt *time.Time `json:"dismissedAt,omitempty"`
	ReopenedAt  *time.Time `json:"reopenedAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type ValidationIssue struct {
	Field   string
	Message string
}

type ValidationError struct {
	Message string
	Details []ValidationIssue
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type RuleConfig struct {
	LargePR             LargePRRuleConfig
	SlowReview          SlowReviewRuleConfig
	Hotspot             HotspotRuleConfig
	DeploymentFailure   DeploymentFailureRuleConfig
	ReviewConcentration ReviewConcentrationRuleConfig
	Bottleneck          BottleneckRuleConfig
	AutoReopen          AutoReopenRuleConfig
	Deduplicate         DeduplicateRuleConfig
}

type LargePRRuleConfig struct {
	Enabled                     bool
	FilesThreshold              int
	TotalChangesThreshold       int
	HighSeverityFilesThreshold  int
	HighSeverityChangeThreshold int
}

type SlowReviewRuleConfig struct {
	Enabled                        bool
	WaitHoursThreshold             float64
	HighSeverityWaitHoursThreshold float64
}

type HotspotRuleConfig struct {
	Enabled                    bool
	ScoreThreshold             int
	HighSeverityScoreThreshold int
	TopFilesLimit              int
}

type DeploymentFailureRuleConfig struct {
	Enabled                 bool
	MinimumDeployments      int
	FailureRateThreshold    float64
	HighSeverityFailureRate float64
}

type ReviewConcentrationRuleConfig struct {
	Enabled                    bool
	MinimumReviewCount         int
	ShareThreshold             float64
	HighSeverityShareThreshold float64
}

type BottleneckRuleConfig struct {
	Enabled                         bool
	MinimumMergedCount              int
	AverageCycleHoursThreshold      float64
	HighSeverityCycleHoursThreshold float64
	StaleOpenCountThreshold         int
	HighSeverityStaleOpenThreshold  int
	StaleOpenAgeDays                int
}

type AutoReopenRuleConfig struct {
	OnReviewed      bool
	OnDismissed     bool
	MinimumSeverity string
}

type DeduplicateRuleConfig struct {
	Enabled bool
	Version int
}
