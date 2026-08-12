package pullrequest

import (
	"context"
	"strings"
)

type store interface {
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
