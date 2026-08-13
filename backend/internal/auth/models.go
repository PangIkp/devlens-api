package auth

import "time"

type LoginRequest struct {
	Email string  `json:"email"`
	Name  *string `json:"name,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type SessionResponse struct {
	AccessToken      string    `json:"accessToken"`
	RefreshToken     string    `json:"refreshToken"`
	TokenType        string    `json:"tokenType"`
	ExpiresAt        time.Time `json:"expiresAt"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	User             User      `json:"user"`
}

type User struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Name      *string    `json:"name,omitempty"`
	AvatarURL *string    `json:"avatarUrl,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type SessionPrincipal struct {
	SessionID string
	User      User
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
