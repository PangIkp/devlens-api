package orgretention

import "time"

// Response reports the effective retention settings for an organization.
type Response struct {
	AnalyticsRawRetentionDays int        `json:"analyticsRawRetentionDays"`
	Enforced                  bool       `json:"enforced"`
	UpdatedAt                 *time.Time `json:"updatedAt,omitempty"`
}

// UpdateRequest sets the organization's raw analytics retention override.
type UpdateRequest struct {
	AnalyticsRawRetentionDays *int `json:"analyticsRawRetentionDays"`
}
