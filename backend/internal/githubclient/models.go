package githubclient

import "time"

type Repository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Archived      bool   `json:"archived"`
}

type User struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

type PullRequest struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	User      User       `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	MergedAt  *time.Time `json:"merged_at"`
}

type Review struct {
	ID          int64      `json:"id"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	User        User       `json:"user"`
	CommitID    string     `json:"commit_id"`
	SubmittedAt *time.Time `json:"submitted_at"`
}

type Commit struct {
	SHA    string       `json:"sha"`
	Author *User        `json:"author"`
	Commit CommitDetail `json:"commit"`
}

type CommitDetail struct {
	Message string       `json:"message"`
	Author  CommitAuthor `json:"author"`
}

type CommitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

type ListOptions struct {
	Page    int
	PerPage int
	State   string
}

type Page[T any] struct {
	Items     []T
	NextPage  int
	RateLimit RateLimit
}

type RateLimit struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
}
