package orgretention

import (
	"context"
	"time"
)

type store interface {
	EnsureOrganizationExists(context.Context, string) error
	Get(ctx context.Context, organizationID string) (*int, *time.Time, error)
	Upsert(ctx context.Context, organizationID string, days *int, updatedBy *string) (*time.Time, error)
}

type Service struct {
	store          store
	defaultRawDays int
}

func NewService(store store, defaultRawDays int) *Service {
	return &Service{store: store, defaultRawDays: defaultRawDays}
}

func (s *Service) Get(ctx context.Context, organizationID string) (Response, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return Response{}, err
	}
	days, updatedAt, err := s.store.Get(ctx, organizationID)
	if err != nil {
		return Response{}, err
	}
	return s.buildResponse(days, updatedAt), nil
}

func (s *Service) Update(ctx context.Context, organizationID string, req UpdateRequest, updatedBy *string) (Response, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return Response{}, err
	}
	if req.AnalyticsRawRetentionDays != nil && *req.AnalyticsRawRetentionDays <= 0 {
		return Response{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "analyticsRawRetentionDays", Message: "must be greater than 0"}},
		}
	}

	updatedAt, err := s.store.Upsert(ctx, organizationID, req.AnalyticsRawRetentionDays, updatedBy)
	if err != nil {
		return Response{}, err
	}
	return s.buildResponse(req.AnalyticsRawRetentionDays, updatedAt), nil
}

func (s *Service) buildResponse(days *int, updatedAt *time.Time) Response {
	effective := s.defaultRawDays
	if days != nil {
		effective = *days
	}
	return Response{
		AnalyticsRawRetentionDays: effective,
		Enforced:                  false,
		UpdatedAt:                 updatedAt,
	}
}
