package organization

import "time"

// CreateOrganizationRequest is the HTTP request model for creating organizations.
type CreateOrganizationRequest struct {
	GithubID int64  `json:"githubId"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
}

// OrganizationResponse is the HTTP response model for organization payloads.
type OrganizationResponse struct {
	ID        string     `json:"id"`
	GithubID  int64      `json:"githubId"`
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt"`
}
