package organizationmember

import (
	"context"
	"strings"
)

type repository interface {
	EnsureOrganizationExists(context.Context, string) error
	EnsureUserExists(context.Context, string) error
	List(context.Context, string) ([]MemberResponse, error)
	Create(context.Context, CreateParams) (MemberResponse, error)
	GetByID(context.Context, string) (MemberResponse, error)
	UpdateRoleWithOwnerGuard(ctx context.Context, params UpdateParams, organizationID string, currentRole string) (MemberResponse, error)
	DeleteWithOwnerGuard(ctx context.Context, organizationID string, memberID string, currentRole string) error
}

type Service struct {
	repository repository
}

func NewService(repository repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, organizationID string) ([]MemberResponse, error) {
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, organizationID)
}

func (s *Service) Create(ctx context.Context, organizationID string, req CreateOrganizationMemberRequest) (MemberResponse, error) {
	if err := validateCreateRequest(req); err != nil {
		return MemberResponse{}, err
	}
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return MemberResponse{}, err
	}
	if err := s.repository.EnsureUserExists(ctx, req.UserID); err != nil {
		return MemberResponse{}, err
	}
	return s.repository.Create(ctx, CreateParams{
		OrganizationID: organizationID,
		UserID:         req.UserID,
		Role:           strings.TrimSpace(req.Role),
	})
}

func (s *Service) Update(ctx context.Context, organizationID string, memberID string, req UpdateOrganizationMemberRequest) (MemberResponse, error) {
	if err := validateUpdateRequest(req); err != nil {
		return MemberResponse{}, err
	}
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return MemberResponse{}, err
	}
	current, err := s.repository.GetByID(ctx, memberID)
	if err != nil {
		return MemberResponse{}, err
	}
	if current.OrganizationID != organizationID {
		return MemberResponse{}, ErrMemberNotFound
	}
	return s.repository.UpdateRoleWithOwnerGuard(ctx, UpdateParams{
		ID:   memberID,
		Role: strings.TrimSpace(*req.Role),
	}, organizationID, current.Role)
}

func (s *Service) Delete(ctx context.Context, organizationID string, memberID string) error {
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return err
	}
	current, err := s.repository.GetByID(ctx, memberID)
	if err != nil {
		return err
	}
	if current.OrganizationID != organizationID {
		return ErrMemberNotFound
	}
	return s.repository.DeleteWithOwnerGuard(ctx, organizationID, memberID, current.Role)
}

func validateCreateRequest(req CreateOrganizationMemberRequest) error {
	var issues []ValidationIssue
	if strings.TrimSpace(req.UserID) == "" {
		issues = append(issues, ValidationIssue{Field: "userId", Message: "is required"})
	}
	if err := validateRole(strings.TrimSpace(req.Role)); err != nil {
		issues = append(issues, *err)
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func validateUpdateRequest(req UpdateOrganizationMemberRequest) error {
	var issues []ValidationIssue
	if req.Role == nil {
		issues = append(issues, ValidationIssue{Field: "role", Message: "is required"})
	} else if err := validateRole(strings.TrimSpace(*req.Role)); err != nil {
		issues = append(issues, *err)
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func validateRole(role string) *ValidationIssue {
	switch Role(role) {
	case RoleOwner, RoleAdmin, RoleMember:
		return nil
	default:
		return &ValidationIssue{Field: "role", Message: "must be one of owner, admin, member"}
	}
}
