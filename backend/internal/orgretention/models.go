package orgretention

import "time"

// Response reports the effective retention settings for an organization.
//
// Enforced is currently always false: the stored override is exposed via
// this API for the frontend to build against, but the raw analytics
// retention window is still applied globally via a ClickHouse table-level
// TTL set at process boot (see internal/clickhouse/schema.go). Per-organization
// enforcement requires a per-row TTL column and ClickHouse mutation strategy
// that has not been built yet.
type Response struct {
	AnalyticsRawRetentionDays int        `json:"analyticsRawRetentionDays"`
	Enforced                  bool       `json:"enforced"`
	UpdatedAt                 *time.Time `json:"updatedAt,omitempty"`
}

// UpdateRequest sets the organization's raw analytics retention override.
type UpdateRequest struct {
	AnalyticsRawRetentionDays *int `json:"analyticsRawRetentionDays"`
}
