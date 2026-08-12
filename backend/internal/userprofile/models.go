package userprofile

import "time"

type Response struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Name      *string    `json:"name,omitempty"`
	AvatarURL *string    `json:"avatarUrl,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
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
