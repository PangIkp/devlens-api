package syncjob

import "time"

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"

	ModeIncremental = "incremental"
	ModeFull        = "full"
)

type CreateSyncRequest struct {
	Mode string  `json:"mode"`
	From *string `json:"from"`
}

type ListParams struct {
	RepositoryID string
	Page         int
	PageSize     int
	Status       string
	SortOrder    string
}

type ListResult struct {
	Items      []SyncJobResponse
	TotalItems int
}

type SyncJobResponse struct {
	ID           string     `json:"id"`
	RepositoryID string     `json:"repositoryId"`
	Status       string     `json:"status"`
	Progress     int        `json:"progress"`
	TriggeredBy  *string    `json:"triggeredBy,omitempty"`
	ErrorMessage *string    `json:"errorMessage,omitempty"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    *time.Time `json:"updatedAt,omitempty"`
}

type createParams struct {
	RepositoryID string
	TriggeredBy  *string
}

type repositoryTarget struct {
	ID           string
	FullName     string
	LastSyncedAt *time.Time
}

type repositoryMetadata struct {
	Name          string
	FullName      string
	DefaultBranch *string
	IsActive      bool
	ArchivedAt    *time.Time
}

type pullRequestInput struct {
	RepositoryID string
	GitHubPRID   int64
	Number       int
	Title        string
	Author       string
	State        string
	IsDraft      bool
	CreatedAt    time.Time
	MergedAt     *time.Time
	ClosedAt     *time.Time
	Additions    int
	Deletions    int
	FilesChanged int
}

type pullRequestReviewInput struct {
	GitHubReviewID    int64
	Reviewer          string
	ReviewRequestedAt *time.Time
	FirstReviewAt     *time.Time
	ReviewSubmittedAt *time.Time
	State             string
}
