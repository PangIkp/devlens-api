package organizationmember

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type CreateOrganizationMemberRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type UpdateOrganizationMemberRequest struct {
	Role *string `json:"role"`
}

type MemberResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	UserID         string `json:"userId"`
	Role           string `json:"role"`
}

type ListParams struct {
	OrganizationID string
}
