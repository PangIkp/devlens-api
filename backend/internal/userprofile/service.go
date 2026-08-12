package userprofile

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

func (s *Service) Get(ctx context.Context, userID string) (Response, error) {
	if err := validateUserID(userID); err != nil {
		return Response{}, err
	}
	return s.repository.GetByID(ctx, strings.TrimSpace(userID))
}

func validateUserID(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "userId", Message: "is required"}},
		}
	}
	parts := strings.Split(userID, "-")
	if len(parts) != 5 {
		return &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "userId", Message: "must be a valid UUID"}},
		}
	}
	return nil
}
