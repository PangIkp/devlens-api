package organization

import (
	"context"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type repository interface {
	Create(context.Context, CreateParams) (OrganizationResponse, error)
	GetByID(context.Context, string) (OrganizationResponse, error)
	List(context.Context, ListParams) (ListResult, error)
	Update(context.Context, UpdateParams) (OrganizationResponse, error)
	SoftDelete(context.Context, string) error
}

// Service owns organization business rules.
type Service struct {
	repository repository
}

func NewService(repository repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(ctx context.Context, userID string, req CreateOrganizationRequest) (OrganizationResponse, error) {
	if err := validateCreateRequest(req); err != nil {
		return OrganizationResponse{}, err
	}

	return s.repository.Create(ctx, CreateParams{
		GithubID:      req.GithubID,
		Slug:          strings.TrimSpace(req.Slug),
		Name:          strings.TrimSpace(req.Name),
		CreatorUserID: strings.TrimSpace(userID),
	})
}

func (s *Service) GetByID(ctx context.Context, id string) (OrganizationResponse, error) {
	return s.repository.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, userID string, params ListParams) (ListResult, error) {
	params.UserID = strings.TrimSpace(userID)
	return s.repository.List(ctx, params)
}

func (s *Service) Update(ctx context.Context, id string, req UpdateOrganizationRequest) (OrganizationResponse, error) {
	current, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return OrganizationResponse{}, err
	}

	merged, err := validateAndMergeUpdateRequest(current, req)
	if err != nil {
		return OrganizationResponse{}, err
	}

	return s.repository.Update(ctx, UpdateParams{
		ID:   id,
		Slug: merged.Slug,
		Name: merged.Name,
	})
}

func (s *Service) SoftDelete(ctx context.Context, id string) error {
	return s.repository.SoftDelete(ctx, id)
}

func validateCreateRequest(req CreateOrganizationRequest) error {
	var issues []ValidationIssue

	if req.GithubID < 1 {
		issues = append(issues, ValidationIssue{
			Field:   "githubId",
			Message: "must be greater than or equal to 1",
		})
	}

	if strings.TrimSpace(req.Name) == "" {
		issues = append(issues, ValidationIssue{
			Field:   "name",
			Message: "is required",
		})
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		issues = append(issues, ValidationIssue{
			Field:   "slug",
			Message: "is required",
		})
	} else if !slugPattern.MatchString(slug) {
		issues = append(issues, ValidationIssue{
			Field:   "slug",
			Message: "must contain only lowercase letters, numbers, and hyphens",
		})
	}

	if len(issues) > 0 {
		return &ValidationError{
			Message: "request validation failed",
			Details: issues,
		}
	}

	return nil
}

func validateAndMergeUpdateRequest(current OrganizationResponse, req UpdateOrganizationRequest) (UpdateParams, error) {
	var issues []ValidationIssue

	if req.Name == nil && req.Slug == nil {
		issues = append(issues, ValidationIssue{
			Field:   "body",
			Message: "must include at least one updatable field",
		})
	}

	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			issues = append(issues, ValidationIssue{
				Field:   "name",
				Message: "is required",
			})
		}
	}

	slug := current.Slug
	if req.Slug != nil {
		slug = strings.TrimSpace(*req.Slug)
		if slug == "" {
			issues = append(issues, ValidationIssue{
				Field:   "slug",
				Message: "is required",
			})
		} else if !slugPattern.MatchString(slug) {
			issues = append(issues, ValidationIssue{
				Field:   "slug",
				Message: "must contain only lowercase letters, numbers, and hyphens",
			})
		}
	}

	if len(issues) > 0 {
		return UpdateParams{}, &ValidationError{
			Message: "request validation failed",
			Details: issues,
		}
	}

	return UpdateParams{
		Slug: slug,
		Name: name,
	}, nil
}
