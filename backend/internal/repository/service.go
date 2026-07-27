package repository

import (
	"context"
	"strings"
	"time"
)

const (
	StatusActive   = "active"
	StatusInactive = "inactive"
	StatusArchived = "archived"
	SortByCreated  = "createdAt"
	SortByName     = "name"
	SortByFullName = "fullName"
	SortOrderAsc   = "asc"
	SortOrderDesc  = "desc"
)

type store interface {
	EnsureOrganizationExists(context.Context, string) error
	Create(context.Context, CreateParams) (RepositoryResponse, error)
	GetByID(context.Context, string) (RepositoryResponse, error)
	List(context.Context, ListParams) (ListResult, error)
	Update(context.Context, UpdateParams) (RepositoryResponse, error)
}

type Service struct {
	repository store
	now        func() time.Time
}

func NewService(repository store) *Service {
	return &Service{
		repository: repository,
		now:        time.Now,
	}
}

func (s *Service) Create(ctx context.Context, organizationID string, req CreateRepositoryRequest) (RepositoryResponse, error) {
	if err := validateCreateRequest(req); err != nil {
		return RepositoryResponse{}, err
	}
	if err := s.repository.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return RepositoryResponse{}, err
	}

	return s.repository.Create(ctx, CreateParams{
		OrganizationID: organizationID,
		GithubID:       req.GithubID,
		Name:           strings.TrimSpace(req.Name),
		FullName:       strings.TrimSpace(req.FullName),
		DefaultBranch:  trimOptionalString(req.DefaultBranch),
	})
}

func (s *Service) GetByID(ctx context.Context, id string) (RepositoryResponse, error) {
	return s.repository.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, params ListParams) (ListResult, error) {
	if err := validateListParams(params); err != nil {
		return ListResult{}, err
	}
	if err := s.repository.EnsureOrganizationExists(ctx, params.OrganizationID); err != nil {
		return ListResult{}, err
	}

	params.Search = strings.TrimSpace(params.Search)
	if params.SortBy == "" {
		params.SortBy = SortByCreated
	}
	if params.SortOrder == "" {
		params.SortOrder = SortOrderDesc
	}
	return s.repository.List(ctx, params)
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRepositoryRequest) (RepositoryResponse, error) {
	current, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return RepositoryResponse{}, err
	}

	merged, err := s.validateAndMergeUpdateRequest(current, req)
	if err != nil {
		return RepositoryResponse{}, err
	}

	return s.repository.Update(ctx, merged)
}

func validateCreateRequest(req CreateRepositoryRequest) error {
	var issues []ValidationIssue

	if req.GithubID < 1 {
		issues = append(issues, ValidationIssue{Field: "githubId", Message: "must be greater than or equal to 1"})
	}
	if strings.TrimSpace(req.Name) == "" {
		issues = append(issues, ValidationIssue{Field: "name", Message: "is required"})
	}
	if strings.TrimSpace(req.FullName) == "" {
		issues = append(issues, ValidationIssue{Field: "fullName", Message: "is required"})
	}
	if !isValidFullName(strings.TrimSpace(req.FullName)) {
		issues = append(issues, ValidationIssue{Field: "fullName", Message: "must be in owner/repository format"})
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func validateListParams(params ListParams) error {
	var issues []ValidationIssue

	if params.Status != "" && params.Status != StatusActive && params.Status != StatusInactive && params.Status != StatusArchived {
		issues = append(issues, ValidationIssue{Field: "status", Message: "must be one of active, inactive, archived"})
	}
	if params.SortBy != "" && params.SortBy != SortByCreated && params.SortBy != SortByName && params.SortBy != SortByFullName {
		issues = append(issues, ValidationIssue{Field: "sortBy", Message: "must be one of createdAt, name, fullName"})
	}
	if params.SortOrder != "" && params.SortOrder != SortOrderAsc && params.SortOrder != SortOrderDesc {
		issues = append(issues, ValidationIssue{Field: "sortOrder", Message: "must be one of asc, desc"})
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func (s *Service) validateAndMergeUpdateRequest(current RepositoryResponse, req UpdateRepositoryRequest) (UpdateParams, error) {
	var issues []ValidationIssue

	if req.Name == nil && req.FullName == nil && req.DefaultBranch == nil && req.IsActive == nil && req.Archived == nil {
		issues = append(issues, ValidationIssue{Field: "body", Message: "must include at least one updatable field"})
	}

	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			issues = append(issues, ValidationIssue{Field: "name", Message: "is required"})
		}
	}

	fullName := current.FullName
	if req.FullName != nil {
		fullName = strings.TrimSpace(*req.FullName)
		if fullName == "" {
			issues = append(issues, ValidationIssue{Field: "fullName", Message: "is required"})
		} else if !isValidFullName(fullName) {
			issues = append(issues, ValidationIssue{Field: "fullName", Message: "must be in owner/repository format"})
		}
	}

	defaultBranch := current.DefaultBranch
	if req.DefaultBranch != nil {
		defaultBranch = trimOptionalString(req.DefaultBranch)
	}

	isActive := current.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	archivedAt := current.ArchivedAt
	if req.Archived != nil {
		if *req.Archived {
			if archivedAt == nil {
				now := s.now().UTC()
				archivedAt = &now
			}
		} else {
			archivedAt = nil
		}
	}

	if len(issues) > 0 {
		return UpdateParams{}, &ValidationError{Message: "request validation failed", Details: issues}
	}

	return UpdateParams{
		ID:            current.ID,
		Name:          name,
		FullName:      fullName,
		DefaultBranch: defaultBranch,
		IsActive:      isActive,
		ArchivedAt:    archivedAt,
	}, nil
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func isValidFullName(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}
