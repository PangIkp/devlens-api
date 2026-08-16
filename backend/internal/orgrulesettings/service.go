package orgrulesettings

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/PangIkp/devlens/backend/internal/insights"
	"github.com/PangIkp/devlens/backend/internal/metrics"
)

type store interface {
	EnsureOrganizationExists(context.Context, string) error
	Get(ctx context.Context, organizationID string) ([]byte, *time.Time, error)
	Upsert(ctx context.Context, organizationID string, configJSON []byte, updatedBy *string) (*time.Time, error)
}

type Service struct {
	store           store
	insightDefaults insights.RuleConfig
	metricsDefaults metrics.RuleConfig
}

func NewService(store store, insightDefaults insights.RuleConfig, metricsDefaults metrics.RuleConfig) *Service {
	return &Service{store: store, insightDefaults: insightDefaults, metricsDefaults: metricsDefaults}
}

func (s *Service) Get(ctx context.Context, organizationID string) (Response, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return Response{}, err
	}
	raw, updatedAt, err := s.store.Get(ctx, organizationID)
	if err != nil {
		return Response{}, err
	}
	stored, err := decodeStoredConfig(raw)
	if err != nil {
		return Response{}, err
	}
	response := buildResponse(stored, s.insightDefaults, s.metricsDefaults)
	response.UpdatedAt = updatedAt
	return response, nil
}

func (s *Service) Update(ctx context.Context, organizationID string, req UpdateRequest, updatedBy *string) (Response, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return Response{}, err
	}
	raw, _, err := s.store.Get(ctx, organizationID)
	if err != nil {
		return Response{}, err
	}
	stored, err := decodeStoredConfig(raw)
	if err != nil {
		return Response{}, err
	}

	merged := mergeStoredConfig(stored, req)
	if err := validateStoredConfig(merged, s.insightDefaults, s.metricsDefaults); err != nil {
		return Response{}, err
	}

	payload, err := json.Marshal(merged)
	if err != nil {
		return Response{}, fmt.Errorf("marshal organization rule settings: %w", err)
	}
	updatedAt, err := s.store.Upsert(ctx, organizationID, payload, updatedBy)
	if err != nil {
		return Response{}, err
	}

	response := buildResponse(merged, s.insightDefaults, s.metricsDefaults)
	response.UpdatedAt = updatedAt
	return response, nil
}

// ResolveInsightRules returns the effective insights.RuleConfig for an
// organization: stored overrides merged over the boot-time defaults. Any
// resolution error falls back to the defaults so callers never fail an
// insight run due to a settings lookup issue.
func (s *Service) ResolveInsightRules(ctx context.Context, organizationID string) (insights.RuleConfig, error) {
	raw, _, err := s.store.Get(ctx, organizationID)
	if err != nil {
		return s.insightDefaults, err
	}
	stored, err := decodeStoredConfig(raw)
	if err != nil {
		return s.insightDefaults, err
	}
	return applyInsightOverrides(stored, s.insightDefaults), nil
}

// ResolveMetricsRules returns the effective metrics.RuleConfig for an
// organization, mirroring ResolveInsightRules.
func (s *Service) ResolveMetricsRules(ctx context.Context, organizationID string) (metrics.RuleConfig, error) {
	raw, _, err := s.store.Get(ctx, organizationID)
	if err != nil {
		return s.metricsDefaults, err
	}
	stored, err := decodeStoredConfig(raw)
	if err != nil {
		return s.metricsDefaults, err
	}
	return applyMetricsOverrides(stored, s.metricsDefaults), nil
}
