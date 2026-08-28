package metrics

import "time"

const (
	IntervalDay          = "day"
	IntervalWeek         = "week"
	IntervalMonth        = "month"
	DayTypeCalendar      = "calendar"
	DayTypeBusiness      = "business"
	CurrentMetricVersion = 1
)

type CalculationRequest struct {
	From          time.Time
	To            time.Time
	MetricVersion int
}

type QueryParams struct {
	From     time.Time
	To       time.Time
	Interval string
	DayType  string
}

type DeploymentQueryParams struct {
	QueryParams
	Environment *string
}

type HotspotQueryParams struct {
	From      time.Time
	To        time.Time
	Page      int
	PageSize  int
	SortOrder string
}

type DataCoverage struct {
	RequestedDays       int     `json:"requestedDays"`
	AvailableDays       int     `json:"availableDays"`
	IsPartial           bool    `json:"isPartial"`
	OldestAvailableDate *string `json:"oldestAvailableDate,omitempty"`
	NewestAvailableDate *string `json:"newestAvailableDate,omitempty"`
}

type DashboardSummary struct {
	MetricVersion       int           `json:"metricVersion"`
	DayType             string        `json:"dayType"`
	RepositoryID        string        `json:"repositoryId"`
	From                string        `json:"from"`
	To                  string        `json:"to"`
	PRCycleTimeMinutes  float64       `json:"prCycleTimeMinutes"`
	ReviewWaitMinutes   float64       `json:"reviewWaitMinutes"`
	DeploymentFrequency float64       `json:"deploymentFrequency"`
	ChangeFailureRate   float64       `json:"changeFailureRate"`
	ReviewCoverage      float64       `json:"reviewCoverage"`
	DataCoverage        *DataCoverage `json:"dataCoverage,omitempty"`
}

type MetricPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type PullRequestMetrics struct {
	MetricVersion           int           `json:"metricVersion"`
	DayType                 string        `json:"dayType"`
	AverageCycleTimeMinutes float64       `json:"averageCycleTimeMinutes"`
	AverageFilesChanged     float64       `json:"averageFilesChanged"`
	AverageAdditions        float64       `json:"averageAdditions"`
	AverageDeletions        float64       `json:"averageDeletions"`
	CycleTimeTrend          []MetricPoint `json:"cycleTimeTrend"`
	DataCoverage            *DataCoverage `json:"dataCoverage,omitempty"`
}

type ReviewMetrics struct {
	MetricVersion        int           `json:"metricVersion"`
	DayType              string        `json:"dayType"`
	AverageWaitMinutes   float64       `json:"averageWaitMinutes"`
	AverageReviewMinutes float64       `json:"averageReviewMinutes"`
	ReviewCoverage       float64       `json:"reviewCoverage"`
	WaitTimeTrend        []MetricPoint `json:"waitTimeTrend"`
	DataCoverage         *DataCoverage `json:"dataCoverage,omitempty"`
}

type DeploymentMetrics struct {
	MetricVersion       int           `json:"metricVersion"`
	DayType             string        `json:"dayType"`
	DeploymentCount     int           `json:"deploymentCount"`
	DeploymentFrequency float64       `json:"deploymentFrequency"`
	ChangeFailureRate   float64       `json:"changeFailureRate"`
	DeploymentTrend     []MetricPoint `json:"deploymentTrend"`
	DataCoverage        *DataCoverage `json:"dataCoverage,omitempty"`
}

type HotspotFile struct {
	FilePath     string  `json:"filePath"`
	HotspotScore float64 `json:"hotspotScore"`
	Additions    int     `json:"additions"`
	Deletions    int     `json:"deletions"`
	CommitCount  int     `json:"commitCount"`
}

type HotspotResult struct {
	Items      []HotspotFile
	TotalItems int
}

type RepositoryMetrics struct {
	MetricVersion int                `json:"metricVersion"`
	DayType       string             `json:"dayType"`
	RepositoryID  string             `json:"repositoryId"`
	From          string             `json:"from"`
	To            string             `json:"to"`
	Interval      string             `json:"interval"`
	Summary       DashboardSummary   `json:"summary"`
	PullRequests  PullRequestMetrics `json:"pullRequests"`
	Reviews       ReviewMetrics      `json:"reviews"`
	Deployments   DeploymentMetrics  `json:"deployments"`
	Hotspots      []HotspotFile      `json:"hotspots"`
}

type ContributorDistributionItem struct {
	Author           string  `json:"author"`
	PullRequestCount int     `json:"pullRequestCount"`
	Share            float64 `json:"share"`
}

type ReviewerDistributionItem struct {
	Reviewer                 string  `json:"reviewer"`
	ReviewCount              int     `json:"reviewCount"`
	ReviewedPullRequestCount int     `json:"reviewedPullRequestCount"`
	Share                    float64 `json:"share"`
}

type WorkloadDistributionSummary struct {
	RepositoryID        string  `json:"repositoryId"`
	From                string  `json:"from"`
	To                  string  `json:"to"`
	TotalPullRequests   int     `json:"totalPullRequests"`
	TotalReviews        int     `json:"totalReviews"`
	TopContributorShare float64 `json:"topContributorShare"`
	TopReviewerShare    float64 `json:"topReviewerShare"`
}

type WorkloadDistribution struct {
	Summary      WorkloadDistributionSummary   `json:"summary"`
	Contributors []ContributorDistributionItem `json:"contributors"`
	Reviewers    []ReviewerDistributionItem    `json:"reviewers"`
}

type ReviewQueueItem struct {
	PullRequestID     string     `json:"pullRequestId"`
	Number            int        `json:"number"`
	Title             string     `json:"title"`
	Author            string     `json:"author"`
	ReviewRequestedAt *time.Time `json:"reviewRequestedAt,omitempty"`
	WaitingMinutes    float64    `json:"waitingMinutes"`
}

type ReviewQueueResult struct {
	Items      []ReviewQueueItem
	TotalItems int
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
