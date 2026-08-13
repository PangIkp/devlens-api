package githubconnection

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/githubapp"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
)

type store interface {
	EnsureOrganizationExists(context.Context, string) error
	GetInstallation(context.Context, string) (*installationRecord, error)
	FindOrganizationIDByInstallationID(context.Context, int64) (*string, error)
	UpsertInstallation(context.Context, string, int64, string, string, string, string, map[string]string, int64, *time.Time) (*installationRecord, error)
	UpdateInstallationLifecycle(context.Context, int64, string, *time.Time, *time.Time) error
	ReplaceAccessibleRepositories(context.Context, string, []accessibleRepositoryRecord) error
	ListAccessibleRepositories(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error)
	GetAccessibleRepositoriesByGitHubIDs(context.Context, string, []int64) ([]accessibleRepositoryRecord, error)
	LinkRepositoryAndMarkSelected(context.Context, string, int64, bool) (string, error)
	ListLinkedRepositoryIDs(context.Context, string) ([]string, error)
}

type syncCreator interface {
	Enqueue(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error)
	ListByRepository(context.Context, syncjob.ListParams) (syncjob.ListResult, error)
	Cancel(context.Context, string) (syncjob.SyncJobResponse, error)
}

type Service struct {
	store    store
	app      githubapp.Client
	syncJobs syncCreator
	now      func() time.Time
}

const installationStateTTL = 30 * time.Minute

var requiredGitHubPermissions = map[string]string{
	"actions":       "read",
	"contents":      "read",
	"deployments":   "read",
	"metadata":      "read",
	"pull_requests": "read",
}

func NewService(store store, app githubapp.Client, syncJobs syncCreator) *Service {
	return &Service{
		store:    store,
		app:      app,
		syncJobs: syncJobs,
		now:      time.Now,
	}
}

func (s *Service) GetConnection(ctx context.Context, organizationID string) (ConnectionResponse, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return ConnectionResponse{}, err
	}

	installation, err := s.store.GetInstallation(ctx, organizationID)
	if err != nil {
		return ConnectionResponse{}, err
	}

	if installation == nil {
		return ConnectionResponse{
			OrganizationID: organizationID,
			Provider:       "github",
			State:          StateNotConnected,
		}, nil
	}

	state := deriveConnectionState(*installation)
	return ConnectionResponse{
		OrganizationID:        organizationID,
		Provider:              "github",
		State:                 state,
		InstallationID:        int64Ptr(installation.InstallationID),
		AccountLogin:          installation.AccountLogin,
		AccountType:           installation.AccountType,
		TargetType:            normalizeTargetType(installation.TargetType),
		ReconnectRequired:     installation.Status == InstallationStatusInstallationRequired,
		LastSyncedAt:          installation.LastSyncedAt,
		LastSyncError:         installation.LastSyncError,
		ConnectedRepositories: installation.ConnectedRepositories,
	}, nil
}

func (s *Service) StartInstallation(ctx context.Context, organizationID string, req StartInstallationRequest) (StartInstallationResponse, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return StartInstallationResponse{}, err
	}
	if s.app == nil || !s.app.Enabled() {
		return StartInstallationResponse{}, ErrConnectionNotConfigured
	}

	state := organizationID + ":" + strconv.FormatInt(s.now().UTC().Unix(), 10)
	installURL, err := s.app.InstallURL(state, trimOptional(req.RedirectURL))
	if err != nil {
		return StartInstallationResponse{}, err
	}
	return StartInstallationResponse{InstallURL: installURL, State: state}, nil
}

func (s *Service) CompleteInstallation(ctx context.Context, organizationID string, installationID int64, state string) (ConnectionResponse, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return ConnectionResponse{}, err
	}
	if s.app == nil || !s.app.Enabled() {
		return ConnectionResponse{}, ErrConnectionNotConfigured
	}
	if err := validateInstallationState(organizationID, state, s.now().UTC()); err != nil {
		return ConnectionResponse{}, err
	}

	installation, err := s.app.GetInstallation(ctx, installationID)
	if err != nil {
		return ConnectionResponse{}, err
	}

	status := deriveInstallationLifecycleStatus(installation)

	targetType := installation.TargetType
	if targetType == "all" {
		targetType = "account"
	} else if targetType == "selected" {
		targetType = "selected_repositories"
	}

	if _, err := s.store.UpsertInstallation(ctx, organizationID, installation.ID, installation.AccountLogin, installation.AccountType, targetType, status, installation.Permissions, installation.InstalledByGithubID, installation.SuspendedAt); err != nil {
		return ConnectionResponse{}, err
	}

	if err := s.refreshAccessibleRepositories(ctx, organizationID, installation.ID, installation.Permissions); err != nil {
		return ConnectionResponse{}, err
	}

	return s.GetConnection(ctx, organizationID)
}

func validateInstallationState(organizationID string, state string, now time.Time) error {
	parts := strings.Split(strings.TrimSpace(state), ":")
	if len(parts) != 2 {
		return ErrInvalidInstallationState
	}
	if parts[0] != strings.TrimSpace(organizationID) {
		return ErrInvalidInstallationState
	}
	issuedAtUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ErrInvalidInstallationState
	}
	issuedAt := time.Unix(issuedAtUnix, 0).UTC()
	if issuedAt.After(now.Add(5 * time.Minute)) {
		return ErrInvalidInstallationState
	}
	if now.Sub(issuedAt) > installationStateTTL {
		return fmt.Errorf("%w: expired", ErrInvalidInstallationState)
	}
	return nil
}

func (s *Service) ListAccessibleRepositories(ctx context.Context, params ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
	if err := s.store.EnsureOrganizationExists(ctx, params.OrganizationID); err != nil {
		return ListAccessibleRepositoriesResult{}, err
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	return s.store.ListAccessibleRepositories(ctx, params)
}

func (s *Service) Disconnect(ctx context.Context, organizationID string) error {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return err
	}

	installation, err := s.store.GetInstallation(ctx, organizationID)
	if err != nil {
		return err
	}
	if installation == nil {
		return ErrInstallationNotFound
	}

	repositoryIDs, err := s.store.ListLinkedRepositoryIDs(ctx, organizationID)
	if err != nil {
		return err
	}

	for _, repositoryID := range repositoryIDs {
		if err := s.cancelActiveSyncJobs(ctx, repositoryID); err != nil {
			return err
		}
	}

	now := s.now().UTC()
	return s.disconnectInstallation(ctx, installation.InstallationID, &now)
}

func (s *Service) cancelActiveSyncJobs(ctx context.Context, repositoryID string) error {
	if s.syncJobs == nil {
		return nil
	}

	jobs, err := s.syncJobs.ListByRepository(ctx, syncjob.ListParams{RepositoryID: repositoryID, Page: 1, PageSize: 20})
	if err != nil {
		return err
	}

	for _, job := range jobs.Items {
		if job.Status != syncjob.StatusPending && job.Status != syncjob.StatusRunning {
			continue
		}
		if _, err := s.syncJobs.Cancel(ctx, job.ID); err != nil && !errors.Is(err, syncjob.ErrSyncJobCancelState) && !errors.Is(err, syncjob.ErrSyncJobNotFound) {
			return err
		}
	}
	return nil
}

func (s *Service) HandleInstallationEvent(ctx context.Context, eventType string, installationID int64, action string) error {
	if installationID < 1 {
		return nil
	}

	switch strings.TrimSpace(eventType) {
	case "installation":
		switch strings.TrimSpace(action) {
		case "deleted":
			now := s.now().UTC()
			return s.disconnectInstallation(ctx, installationID, &now)
		case "suspend":
			now := s.now().UTC()
			return s.store.UpdateInstallationLifecycle(ctx, installationID, StateInstallationRequired, &now, nil)
		case "unsuspend", "created", "new_permissions_accepted":
			return s.refreshInstallationByInstallationID(ctx, installationID)
		default:
			return s.refreshInstallationByInstallationID(ctx, installationID)
		}
	case "installation_repositories":
		return s.refreshInstallationByInstallationID(ctx, installationID)
	default:
		return nil
	}
}

func (s *Service) SelectRepositories(ctx context.Context, organizationID string, req SelectRepositoriesRequest) (SelectRepositoriesResponse, error) {
	if err := s.store.EnsureOrganizationExists(ctx, organizationID); err != nil {
		return SelectRepositoriesResponse{}, err
	}
	if err := validateSelectRequest(req); err != nil {
		return SelectRepositoriesResponse{}, err
	}

	installationRepos, err := s.store.GetAccessibleRepositoriesByGitHubIDs(ctx, organizationID, req.RepositoryIDs)
	if err != nil {
		return SelectRepositoriesResponse{}, err
	}
	if len(installationRepos) != len(req.RepositoryIDs) {
		return SelectRepositoriesResponse{}, ErrAccessibleRepositoryNotFound
	}

	autoSync := req.AutoSync == nil || *req.AutoSync
	result := SelectRepositoriesResponse{
		State:                 StateConnected,
		SelectedRepositoryIDs: make([]int64, 0, len(installationRepos)),
		CreatedRepositoryIDs:  make([]string, 0, len(installationRepos)),
		SyncJobIDs:            make([]string, 0, len(installationRepos)),
	}

	if autoSync {
		result.State = StateSyncing
	}

	for _, repo := range installationRepos {
		if repo.InstallationStatus != InstallationStatusAccessible {
			return SelectRepositoriesResponse{}, &ValidationError{
				Message: "request validation failed",
				Details: []ValidationIssue{{Field: "repositoryIds", Message: "contains repositories without accessible installation permissions"}},
			}
		}

		repositoryID, err := s.store.LinkRepositoryAndMarkSelected(ctx, organizationID, repo.GithubRepositoryID, autoSync)
		if err != nil {
			return SelectRepositoriesResponse{}, err
		}
		result.SelectedRepositoryIDs = append(result.SelectedRepositoryIDs, repo.GithubRepositoryID)
		result.CreatedRepositoryIDs = append(result.CreatedRepositoryIDs, repositoryID)

		if autoSync && s.syncJobs != nil {
			job, err := s.syncJobs.Enqueue(ctx, repositoryID, syncjob.CreateSyncRequest{Mode: syncjob.ModeFull})
			if err != nil {
				result.State = StateSyncFailed
				continue
			}
			result.SyncJobIDs = append(result.SyncJobIDs, job.ID)
		}
	}

	return result, nil
}

func (s *Service) refreshAccessibleRepositories(ctx context.Context, organizationID string, installationID int64, permissions map[string]string) error {
	items := make([]accessibleRepositoryRecord, 0)
	installationStatus := deriveRepositoryInstallationStatus(permissions)
	page := 1
	for {
		result, err := s.app.ListInstallationRepositories(ctx, installationID, page, 100)
		if err != nil {
			return err
		}
		for _, item := range result.Items {
			items = append(items, accessibleRepositoryRecord{
				GithubRepositoryID: item.GithubRepositoryID,
				Name:               item.Name,
				OwnerLogin:         item.OwnerLogin,
				FullName:           item.FullName,
				Private:            item.Private,
				DefaultBranch:      item.DefaultBranch,
				InstallationStatus: installationStatus,
			})
		}
		if result.NextPage == 0 {
			break
		}
		page = result.NextPage
	}
	return s.store.ReplaceAccessibleRepositories(ctx, organizationID, items)
}

func deriveConnectionState(installation installationRecord) string {
	if installation.SuspendedAt != nil || installation.Status == StateInstallationRequired {
		return StateInstallationRequired
	}
	if installation.SyncingRepositories > 0 {
		return StateSyncing
	}
	if installation.FailedRepositories > 0 {
		return StateSyncFailed
	}
	if installation.LastSyncError != nil {
		return StateSyncFailed
	}
	if installation.LastSyncedAt != nil {
		return StateConnected
	}
	if installation.ConnectedRepositories > 0 {
		return StateConnected
	}
	return StateConnected
}

func (s *Service) refreshInstallationByInstallationID(ctx context.Context, installationID int64) error {
	if s.app == nil || !s.app.Enabled() {
		return ErrConnectionNotConfigured
	}

	organizationID, err := s.store.FindOrganizationIDByInstallationID(ctx, installationID)
	if err != nil || organizationID == nil {
		return err
	}

	installation, err := s.app.GetInstallation(ctx, installationID)
	if err != nil {
		return err
	}

	status := deriveInstallationLifecycleStatus(installation)

	targetType := installation.TargetType
	if targetType == "all" {
		targetType = "account"
	} else if targetType == "selected" {
		targetType = "selected_repositories"
	}

	if _, err := s.store.UpsertInstallation(ctx, *organizationID, installation.ID, installation.AccountLogin, installation.AccountType, targetType, status, installation.Permissions, installation.InstalledByGithubID, installation.SuspendedAt); err != nil {
		return err
	}

	return s.refreshAccessibleRepositories(ctx, *organizationID, installation.ID, installation.Permissions)
}

func (s *Service) disconnectInstallation(ctx context.Context, installationID int64, disconnectedAt *time.Time) error {
	organizationID, err := s.store.FindOrganizationIDByInstallationID(ctx, installationID)
	if err != nil || organizationID == nil {
		return err
	}

	if err := s.store.UpdateInstallationLifecycle(ctx, installationID, StateInstallationRequired, nil, disconnectedAt); err != nil {
		return err
	}

	return s.store.ReplaceAccessibleRepositories(ctx, *organizationID, nil)
}

func normalizeTargetType(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	switch trimmed {
	case "all":
		trimmed = "account"
	case "selected":
		trimmed = "selected_repositories"
	}
	return &trimmed
}

func validateSelectRequest(req SelectRepositoriesRequest) error {
	var issues []ValidationIssue
	if len(req.RepositoryIDs) == 0 {
		issues = append(issues, ValidationIssue{Field: "repositoryIds", Message: "must include at least one repository id"})
	}
	for _, id := range req.RepositoryIDs {
		if id < 1 {
			issues = append(issues, ValidationIssue{Field: "repositoryIds", Message: "must contain positive integers"})
			break
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func trimOptional(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func int64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	copy := value
	return &copy
}

func deriveInstallationLifecycleStatus(installation githubapp.Installation) string {
	if installation.SuspendedAt != nil || !hasRequiredPermissions(installation.Permissions) {
		return StateInstallationRequired
	}
	return StateConnected
}

func deriveRepositoryInstallationStatus(permissions map[string]string) string {
	if !hasRequiredPermissions(permissions) {
		return InstallationStatusPermissionMissing
	}
	return InstallationStatusAccessible
}

func hasRequiredPermissions(permissions map[string]string) bool {
	for name, level := range requiredGitHubPermissions {
		if !permissionSatisfies(permissions[name], level) {
			return false
		}
	}
	return true
}

func permissionSatisfies(actual string, required string) bool {
	actual = strings.TrimSpace(strings.ToLower(actual))
	required = strings.TrimSpace(strings.ToLower(required))
	switch required {
	case "read":
		return actual == "read" || actual == "write"
	case "write":
		return actual == "write"
	default:
		return actual == required
	}
}
