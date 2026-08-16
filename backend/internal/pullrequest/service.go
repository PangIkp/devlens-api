package pullrequest

import (
	"context"
	"strings"
)

type store interface {
	EnsureRepositoryExists(context.Context, string) error
	List(context.Context, ListParams) (ListResult, error)
	GetByID(context.Context, string) (Response, error)
}

type Service struct {
	repository store
}

func NewService(repository store) *Service {
	return &Service{repository: repository}
}

func (s *Service) GetByID(ctx context.Context, id string) (Response, error) {
	if err := validateID(id); err != nil {
		return Response{}, err
	}
	return s.repository.GetByID(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, params ListParams) (ListResult, error) {
	if err := validateListParams(params); err != nil {
		return ListResult{}, err
	}
	params.RepositoryID = strings.TrimSpace(params.RepositoryID)
	params.Search = strings.TrimSpace(params.Search)
	if params.SortBy == "" {
		params.SortBy = "createdAt"
	}
	if params.SortOrder == "" {
		params.SortOrder = "desc"
	}
	if err := s.repository.EnsureRepositoryExists(ctx, params.RepositoryID); err != nil {
		return ListResult{}, err
	}
	return s.repository.List(ctx, params)
}

func validateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "pullRequestId", Message: "is required"}},
		}
	}
	if len(strings.Split(id, "-")) != 5 {
		return &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "pullRequestId", Message: "must be a valid UUID"}},
		}
	}
	return nil
}

func validateListParams(params ListParams) error {
	var issues []ValidationIssue
	repositoryID := strings.TrimSpace(params.RepositoryID)
	if repositoryID == "" {
		issues = append(issues, ValidationIssue{Field: "repositoryId", Message: "is required"})
	} else if len(strings.Split(repositoryID, "-")) != 5 {
		issues = append(issues, ValidationIssue{Field: "repositoryId", Message: "must be a valid UUID"})
	}
	if params.Page < 1 {
		issues = append(issues, ValidationIssue{Field: "page", Message: "must be greater than or equal to 1"})
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		issues = append(issues, ValidationIssue{Field: "pageSize", Message: "must be between 1 and 100"})
	}
	if params.Status != "" && params.Status != "open" && params.Status != "closed" && params.Status != "merged" {
		issues = append(issues, ValidationIssue{Field: "status", Message: "must be one of open, closed, merged"})
	}
	if params.SortBy != "" && params.SortBy != "createdAt" && params.SortBy != "number" {
		issues = append(issues, ValidationIssue{Field: "sortBy", Message: "must be one of createdAt, number"})
	}
	if params.SortOrder != "" && params.SortOrder != "asc" && params.SortOrder != "desc" {
		issues = append(issues, ValidationIssue{Field: "sortOrder", Message: "must be one of asc, desc"})
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}
