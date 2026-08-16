package orgretention

import "errors"

var ErrOrganizationNotFound = errors.New("organization not found")

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
