package organization

// CreateOrganizationRequest is the HTTP request model for creating organizations.
type CreateOrganizationRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// OrganizationResponse is the HTTP response model for organization payloads.
type OrganizationResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}
