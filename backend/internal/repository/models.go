package repository

import "time"

type CreateRepositoryRequest struct {
	GithubID      int64   `json:"githubId"`
	Name          string  `json:"name"`
	FullName      string  `json:"fullName"`
	DefaultBranch *string `json:"defaultBranch"`
}

type UpdateRepositoryRequest struct {
	Name          *string `json:"name"`
	FullName      *string `json:"fullName"`
	DefaultBranch *string `json:"defaultBranch"`
	IsActive      *bool   `json:"isActive"`
	Archived      *bool   `json:"archived"`
}

type RepositoryResponse struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	GithubID       int64      `json:"githubId"`
	Name           string     `json:"name"`
	FullName       string     `json:"fullName"`
	DefaultBranch  *string    `json:"defaultBranch,omitempty"`
	IsActive       bool       `json:"isActive"`
	ArchivedAt     *time.Time `json:"archivedAt"`
	LastSyncedAt   *time.Time `json:"lastSyncedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      *time.Time `json:"updatedAt"`
}

type ListParams struct {
	OrganizationID string
	Page           int
	PageSize       int
	Status         string
	Search         string
	SortBy         string
	SortOrder      string
}

type ListResult struct {
	Items      []RepositoryResponse
	TotalItems int
}
