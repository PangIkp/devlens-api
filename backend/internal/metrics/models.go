package metrics

import "time"

const (
	IntervalDay   = "day"
	IntervalWeek  = "week"
	IntervalMonth = "month"
)

type CalculationRequest struct {
	From time.Time
	To   time.Time
}

type QueryParams struct {
	From     time.Time
	To       time.Time
	Interval string
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

type DashboardSummary struct {
	RepositoryID        string  `json:"repositoryId"`
	From                string  `json:"from"`
	To                  string  `json:"to"`
	PRCycleTimeMinutes  float64 `json:"prCycleTimeMinutes"`
	ReviewWaitMinutes   float64 `json:"reviewWaitMinutes"`
	DeploymentFrequency float64 `json:"deploymentFrequency"`
	ChangeFailureRate   float64 `json:"changeFailureRate"`
	ReviewCoverage      float64 `json:"reviewCoverage"`
}

type MetricPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type PullRequestMetrics struct {
	AverageCycleTimeMinutes float64       `json:"averageCycleTimeMinutes"`
	AverageFilesChanged     float64       `json:"averageFilesChanged"`
	AverageAdditions        float64       `json:"averageAdditions"`
	AverageDeletions        float64       `json:"averageDeletions"`
	CycleTimeTrend          []MetricPoint `json:"cycleTimeTrend"`
}

type ReviewMetrics struct {
	AverageWaitMinutes   float64       `json:"averageWaitMinutes"`
	AverageReviewMinutes float64       `json:"averageReviewMinutes"`
	ReviewCoverage       float64       `json:"reviewCoverage"`
	WaitTimeTrend        []MetricPoint `json:"waitTimeTrend"`
}

type DeploymentMetrics struct {
	DeploymentCount     int           `json:"deploymentCount"`
	DeploymentFrequency float64       `json:"deploymentFrequency"`
	ChangeFailureRate   float64       `json:"changeFailureRate"`
	DeploymentTrend     []MetricPoint `json:"deploymentTrend"`
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
	RepositoryID string             `json:"repositoryId"`
	From         string             `json:"from"`
	To           string             `json:"to"`
	Interval     string             `json:"interval"`
	Summary      DashboardSummary   `json:"summary"`
	PullRequests PullRequestMetrics `json:"pullRequests"`
	Reviews      ReviewMetrics      `json:"reviews"`
	Deployments  DeploymentMetrics  `json:"deployments"`
	Hotspots     []HotspotFile      `json:"hotspots"`
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
