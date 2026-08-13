package authorization

import "context"

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type store interface {
	GetOrganizationRole(context.Context, string, string) (string, error)
	GetRepositoryRole(context.Context, string, string) (string, error)
	GetPullRequestRole(context.Context, string, string) (string, error)
	GetSyncJobRole(context.Context, string, string) (string, error)
}

type Service struct {
	repository store
}

func NewService(repository store) *Service {
	return &Service{repository: repository}
}

func (s *Service) AuthorizeOrganization(ctx context.Context, userID string, organizationID string, roles ...string) error {
	return authorize(ctx, s.repository.GetOrganizationRole, userID, organizationID, roles...)
}

func (s *Service) AuthorizeRepository(ctx context.Context, userID string, repositoryID string, roles ...string) error {
	return authorize(ctx, s.repository.GetRepositoryRole, userID, repositoryID, roles...)
}

func (s *Service) AuthorizePullRequest(ctx context.Context, userID string, pullRequestID string, roles ...string) error {
	return authorize(ctx, s.repository.GetPullRequestRole, userID, pullRequestID, roles...)
}

func (s *Service) AuthorizeSyncJob(ctx context.Context, userID string, syncJobID string, roles ...string) error {
	return authorize(ctx, s.repository.GetSyncJobRole, userID, syncJobID, roles...)
}

func authorize(ctx context.Context, loadRole func(context.Context, string, string) (string, error), userID string, resourceID string, roles ...string) error {
	role, err := loadRole(ctx, userID, resourceID)
	if err != nil {
		return err
	}
	for _, candidate := range roles {
		if role == candidate {
			return nil
		}
	}
	return ErrForbidden
}
