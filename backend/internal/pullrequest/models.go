package pullrequest

import "time"

type Review struct {
	ID                string     `json:"id"`
	GithubReviewID    int64      `json:"githubReviewId"`
	Reviewer          string     `json:"reviewer"`
	State             string     `json:"state"`
	ReviewRequestedAt *time.Time `json:"reviewRequestedAt,omitempty"`
	FirstReviewAt     *time.Time `json:"firstReviewAt,omitempty"`
	ReviewSubmittedAt *time.Time `json:"reviewSubmittedAt,omitempty"`
}

type FileChange struct {
	ID          string `json:"id"`
	FilePath    string `json:"filePath"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	CommitCount int    `json:"commitCount"`
}

type TimelineEvent struct {
	Type       string    `json:"type"`
	Label      string    `json:"label"`
	Actor      *string   `json:"actor,omitempty"`
	State      *string   `json:"state,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
}

type RiskIndicator struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

type RepositoryRef struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
}

type ListItem struct {
	ID           string        `json:"id"`
	Repository   RepositoryRef `json:"repository"`
	GithubPRID   int64         `json:"githubPrId"`
	Number       int           `json:"number"`
	Title        string        `json:"title"`
	Author       string        `json:"author"`
	State        string        `json:"state"`
	CreatedAt    time.Time     `json:"createdAt"`
	MergedAt     *time.Time    `json:"mergedAt,omitempty"`
	ClosedAt     *time.Time    `json:"closedAt,omitempty"`
	Additions    int           `json:"additions"`
	Deletions    int           `json:"deletions"`
	FilesChanged int           `json:"filesChanged"`
	IsDraft      bool          `json:"isDraft"`
}

type ListParams struct {
	RepositoryID string
	Page         int
	PageSize     int
	Status       string
	Search       string
	SortBy       string
	SortOrder    string
}

type ListResult struct {
	Items      []ListItem
	TotalItems int
}

type Response struct {
	ID            string          `json:"id"`
	Repository    RepositoryRef   `json:"repository"`
	GithubPRID    int64           `json:"githubPrId"`
	Number        int             `json:"number"`
	Title         string          `json:"title"`
	Author        string          `json:"author"`
	State         string          `json:"state"`
	CreatedAt     time.Time       `json:"createdAt"`
	MergedAt      *time.Time      `json:"mergedAt,omitempty"`
	ClosedAt      *time.Time      `json:"closedAt,omitempty"`
	Additions     int             `json:"additions"`
	Deletions     int             `json:"deletions"`
	FilesChanged  int             `json:"filesChanged"`
	IsDraft       bool            `json:"isDraft"`
	Reviews       []Review        `json:"reviews"`
	FileChanges   []FileChange    `json:"fileChanges"`
	Timeline      []TimelineEvent `json:"timeline"`
	RiskIndicator RiskIndicator   `json:"riskIndicator"`
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
