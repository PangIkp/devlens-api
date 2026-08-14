package githubconnection

import "errors"

var (
	ErrOrganizationNotFound           = errors.New("organization not found")
	ErrInstallationNotFound           = errors.New("github installation not found")
	ErrConnectionNotConfigured        = errors.New("github app is not configured")
	ErrAccessibleRepositoryNotFound   = errors.New("accessible repository not found")
	ErrInvalidInstallationState       = errors.New("github installation state is invalid")
	ErrInstallationLinkedToAnotherOrg = errors.New("github installation is already connected to a different organization")
)

type ValidationError struct {
	Message string
	Details []ValidationIssue
}

type ValidationIssue struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}
