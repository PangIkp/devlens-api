package githubconnection

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/githubapp"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
)

type stubStore struct {
	ensureFn          func(context.Context, string) error
	getInstallationFn func(context.Context, string) (*installationRecord, error)
	findOrgFn         func(context.Context, int64) (*string, error)
	upsertFn          func(context.Context, string, int64, string, string, string, string, map[string]string, int64, *time.Time) (*installationRecord, error)
	updateLifeFn      func(context.Context, int64, string, *time.Time, *time.Time) error
	replaceFn         func(context.Context, string, []accessibleRepositoryRecord) error
	listFn            func(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error)
	getReposFn        func(context.Context, string, []int64) ([]accessibleRepositoryRecord, error)
	linkFn            func(context.Context, string, int64, bool) (string, error)
}

func (s stubStore) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	return s.ensureFn(ctx, organizationID)
}
func (s stubStore) GetInstallation(ctx context.Context, organizationID string) (*installationRecord, error) {
	return s.getInstallationFn(ctx, organizationID)
}
func (s stubStore) FindOrganizationIDByInstallationID(ctx context.Context, installationID int64) (*string, error) {
	return s.findOrgFn(ctx, installationID)
}
func (s stubStore) UpsertInstallation(ctx context.Context, organizationID string, installationID int64, accountLogin, accountType, targetType, status string, permissions map[string]string, installedByGitHubID int64, suspendedAt *time.Time) (*installationRecord, error) {
	return s.upsertFn(ctx, organizationID, installationID, accountLogin, accountType, targetType, status, permissions, installedByGitHubID, suspendedAt)
}
func (s stubStore) UpdateInstallationLifecycle(ctx context.Context, installationID int64, status string, suspendedAt *time.Time, disconnectedAt *time.Time) error {
	return s.updateLifeFn(ctx, installationID, status, suspendedAt, disconnectedAt)
}
func (s stubStore) ReplaceAccessibleRepositories(ctx context.Context, organizationID string, items []accessibleRepositoryRecord) error {
	return s.replaceFn(ctx, organizationID, items)
}
func (s stubStore) ListAccessibleRepositories(ctx context.Context, params ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
	return s.listFn(ctx, params)
}
func (s stubStore) GetAccessibleRepositoriesByGitHubIDs(ctx context.Context, organizationID string, ids []int64) ([]accessibleRepositoryRecord, error) {
	return s.getReposFn(ctx, organizationID, ids)
}
func (s stubStore) LinkRepositoryAndMarkSelected(ctx context.Context, organizationID string, githubRepositoryID int64, autoSync bool) (string, error) {
	return s.linkFn(ctx, organizationID, githubRepositoryID, autoSync)
}

type stubApp struct {
	enabled      bool
	installURLFn func(string, string) (string, error)
	getFn        func(context.Context, int64) (githubapp.Installation, error)
	tokenFn      func(context.Context, int64) (githubapp.Token, error)
	listFn       func(context.Context, int64, int, int) (githubapp.RepositoryPage, error)
}

func (s stubApp) Enabled() bool { return s.enabled }
func (s stubApp) InstallURL(state string, redirectURL string) (string, error) {
	return s.installURLFn(state, redirectURL)
}
func (s stubApp) GetInstallation(ctx context.Context, installationID int64) (githubapp.Installation, error) {
	return s.getFn(ctx, installationID)
}
func (s stubApp) CreateInstallationToken(ctx context.Context, installationID int64) (githubapp.Token, error) {
	return s.tokenFn(ctx, installationID)
}
func (s stubApp) ListInstallationRepositories(ctx context.Context, installationID int64, page, perPage int) (githubapp.RepositoryPage, error) {
	return s.listFn(ctx, installationID, page, perPage)
}

type stubSyncCreator struct {
	enqueueFn func(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error)
}

func (s stubSyncCreator) Enqueue(ctx context.Context, repositoryID string, req syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
	return s.enqueueFn(ctx, repositoryID, req)
}

func TestGetConnectionReturnsNotConnectedWithoutInstallation(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		ensureFn:          func(context.Context, string) error { return nil },
		getInstallationFn: func(context.Context, string) (*installationRecord, error) { return nil, nil },
		findOrgFn:         func(context.Context, int64) (*string, error) { return nil, nil },
		upsertFn: func(context.Context, string, int64, string, string, string, string, map[string]string, int64, *time.Time) (*installationRecord, error) {
			return nil, nil
		},
		updateLifeFn: func(context.Context, int64, string, *time.Time, *time.Time) error { return nil },
		replaceFn:    func(context.Context, string, []accessibleRepositoryRecord) error { return nil },
		listFn: func(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
			return ListAccessibleRepositoriesResult{}, nil
		},
		getReposFn: func(context.Context, string, []int64) ([]accessibleRepositoryRecord, error) { return nil, nil },
		linkFn:     func(context.Context, string, int64, bool) (string, error) { return "", nil },
	}, nil, nil)

	result, err := service.GetConnection(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.State != StateNotConnected {
		t.Fatalf("expected state %q, got %q", StateNotConnected, result.State)
	}
}

func TestSelectRepositoriesCreatesSyncJobs(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		ensureFn: func(context.Context, string) error { return nil },
		getInstallationFn: func(context.Context, string) (*installationRecord, error) {
			return &installationRecord{ID: "inst-1", InstallationID: 42, Status: StateConnected}, nil
		},
		findOrgFn: func(context.Context, int64) (*string, error) { return nil, nil },
		upsertFn: func(context.Context, string, int64, string, string, string, string, map[string]string, int64, *time.Time) (*installationRecord, error) {
			return nil, nil
		},
		updateLifeFn: func(context.Context, int64, string, *time.Time, *time.Time) error { return nil },
		replaceFn:    func(context.Context, string, []accessibleRepositoryRecord) error { return nil },
		listFn: func(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
			return ListAccessibleRepositoriesResult{}, nil
		},
		getReposFn: func(context.Context, string, []int64) ([]accessibleRepositoryRecord, error) {
			return []accessibleRepositoryRecord{
				{GithubRepositoryID: 1, InstallationStatus: InstallationStatusAccessible},
			}, nil
		},
		linkFn: func(context.Context, string, int64, bool) (string, error) {
			return "repo-1", nil
		},
	}, nil, stubSyncCreator{
		enqueueFn: func(_ context.Context, repositoryID string, req syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error) {
			if repositoryID != "repo-1" {
				t.Fatalf("unexpected repository id %q", repositoryID)
			}
			if req.Mode != syncjob.ModeFull {
				t.Fatalf("expected full sync mode, got %q", req.Mode)
			}
			return syncjob.SyncJobResponse{ID: "job-1"}, nil
		},
	})

	result, err := service.SelectRepositories(context.Background(), "org-1", SelectRepositoriesRequest{
		RepositoryIDs: []int64{1},
		AutoSync:      boolPtr(true),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.State != StateSyncing {
		t.Fatalf("expected state %q, got %q", StateSyncing, result.State)
	}
	if len(result.SyncJobIDs) != 1 || result.SyncJobIDs[0] != "job-1" {
		t.Fatalf("unexpected sync job ids %#v", result.SyncJobIDs)
	}
}

func TestHandleInstallationEventIgnoresUnknownInstallationForOutOfOrderCallback(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		ensureFn:          func(context.Context, string) error { return nil },
		getInstallationFn: func(context.Context, string) (*installationRecord, error) { return nil, nil },
		findOrgFn:         func(context.Context, int64) (*string, error) { return nil, nil },
		upsertFn: func(context.Context, string, int64, string, string, string, string, map[string]string, int64, *time.Time) (*installationRecord, error) {
			t.Fatal("upsert should not run before installation is linked to an organization")
			return nil, nil
		},
		updateLifeFn: func(context.Context, int64, string, *time.Time, *time.Time) error {
			t.Fatal("lifecycle update should not run for unknown installation")
			return nil
		},
		replaceFn: func(context.Context, string, []accessibleRepositoryRecord) error {
			t.Fatal("repository refresh should not run for unknown installation")
			return nil
		},
		listFn: func(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
			return ListAccessibleRepositoriesResult{}, nil
		},
		getReposFn: func(context.Context, string, []int64) ([]accessibleRepositoryRecord, error) { return nil, nil },
		linkFn:     func(context.Context, string, int64, bool) (string, error) { return "", nil },
	}, stubApp{
		enabled: true,
		getFn: func(context.Context, int64) (githubapp.Installation, error) {
			t.Fatal("github app fetch should not run for unknown installation")
			return githubapp.Installation{}, nil
		},
		listFn: func(context.Context, int64, int, int) (githubapp.RepositoryPage, error) {
			t.Fatal("installation repositories fetch should not run for unknown installation")
			return githubapp.RepositoryPage{}, nil
		},
	}, nil)

	if err := service.HandleInstallationEvent(context.Background(), "installation", 42, "created"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCompleteInstallationMarksPermissionGap(t *testing.T) {
	t.Parallel()

	var storedStatus string
	var repositoryStatus string

	service := NewService(stubStore{
		ensureFn:          func(context.Context, string) error { return nil },
		getInstallationFn: func(context.Context, string) (*installationRecord, error) { return nil, nil },
		findOrgFn:         func(context.Context, int64) (*string, error) { return nil, nil },
		upsertFn: func(_ context.Context, _ string, _ int64, _ string, _ string, _ string, status string, _ map[string]string, _ int64, _ *time.Time) (*installationRecord, error) {
			storedStatus = status
			return &installationRecord{InstallationID: 42, Status: status}, nil
		},
		updateLifeFn: func(context.Context, int64, string, *time.Time, *time.Time) error { return nil },
		replaceFn: func(_ context.Context, _ string, items []accessibleRepositoryRecord) error {
			if len(items) != 1 {
				t.Fatalf("expected one repository, got %d", len(items))
			}
			repositoryStatus = items[0].InstallationStatus
			return nil
		},
		listFn: func(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
			return ListAccessibleRepositoriesResult{}, nil
		},
		getReposFn: func(context.Context, string, []int64) ([]accessibleRepositoryRecord, error) { return nil, nil },
		linkFn:     func(context.Context, string, int64, bool) (string, error) { return "", nil },
	}, stubApp{
		enabled: true,
		getFn: func(context.Context, int64) (githubapp.Installation, error) {
			return githubapp.Installation{
				ID:           42,
				AccountLogin: "devlens",
				AccountType:  "User",
				TargetType:   "all",
				Permissions: map[string]string{
					"metadata": "read",
				},
			}, nil
		},
		listFn: func(context.Context, int64, int, int) (githubapp.RepositoryPage, error) {
			return githubapp.RepositoryPage{
				Items: []githubapp.AccessibleRepository{
					{GithubRepositoryID: 1, Name: "repo", OwnerLogin: "devlens", FullName: "devlens/repo"},
				},
			}, nil
		},
	}, nil)

	validState := "org-1:" + strconv.FormatInt(time.Now().UTC().Unix(), 10)
	_, err := service.CompleteInstallation(context.Background(), "org-1", 42, validState)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if storedStatus != StateInstallationRequired {
		t.Fatalf("expected stored status %q, got %q", StateInstallationRequired, storedStatus)
	}
	if repositoryStatus != InstallationStatusPermissionMissing {
		t.Fatalf("expected repository status %q, got %q", InstallationStatusPermissionMissing, repositoryStatus)
	}
}

func TestCompleteInstallationRejectsExpiredState(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{
		ensureFn:          func(context.Context, string) error { return nil },
		getInstallationFn: func(context.Context, string) (*installationRecord, error) { return nil, nil },
		findOrgFn:         func(context.Context, int64) (*string, error) { return nil, nil },
		upsertFn: func(context.Context, string, int64, string, string, string, string, map[string]string, int64, *time.Time) (*installationRecord, error) {
			t.Fatal("upsert should not run for expired state")
			return nil, nil
		},
		updateLifeFn: func(context.Context, int64, string, *time.Time, *time.Time) error { return nil },
		replaceFn:    func(context.Context, string, []accessibleRepositoryRecord) error { return nil },
		listFn: func(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
			return ListAccessibleRepositoriesResult{}, nil
		},
		getReposFn: func(context.Context, string, []int64) ([]accessibleRepositoryRecord, error) { return nil, nil },
		linkFn:     func(context.Context, string, int64, bool) (string, error) { return "", nil },
	}, stubApp{
		enabled: true,
		getFn: func(context.Context, int64) (githubapp.Installation, error) {
			t.Fatal("github app fetch should not run for expired state")
			return githubapp.Installation{}, nil
		},
	}, nil)
	service.now = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

	_, err := service.CompleteInstallation(context.Background(), "org-1", 42, "org-1:1699990000")
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrInvalidInstallationState && !strings.Contains(err.Error(), ErrInvalidInstallationState.Error()) {
		t.Fatalf("expected invalid state error, got %v", err)
	}
}

func TestHandleInstallationEventDeletedDisconnectsAndClearsRepositories(t *testing.T) {
	t.Parallel()

	organizationID := "org-1"
	var gotStatus string
	var gotDisconnectedAt *time.Time
	var clearedRepositoryCount int

	service := NewService(stubStore{
		ensureFn:          func(context.Context, string) error { return nil },
		getInstallationFn: func(context.Context, string) (*installationRecord, error) { return nil, nil },
		findOrgFn: func(context.Context, int64) (*string, error) {
			return &organizationID, nil
		},
		upsertFn: func(context.Context, string, int64, string, string, string, string, map[string]string, int64, *time.Time) (*installationRecord, error) {
			return nil, nil
		},
		updateLifeFn: func(_ context.Context, _ int64, status string, _ *time.Time, disconnectedAt *time.Time) error {
			gotStatus = status
			gotDisconnectedAt = disconnectedAt
			return nil
		},
		replaceFn: func(_ context.Context, _ string, items []accessibleRepositoryRecord) error {
			clearedRepositoryCount = len(items)
			return nil
		},
		listFn: func(context.Context, ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
			return ListAccessibleRepositoriesResult{}, nil
		},
		getReposFn: func(context.Context, string, []int64) ([]accessibleRepositoryRecord, error) { return nil, nil },
		linkFn:     func(context.Context, string, int64, bool) (string, error) { return "", nil },
	}, nil, nil)

	if err := service.HandleInstallationEvent(context.Background(), "installation", 42, "deleted"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotStatus != StateInstallationRequired {
		t.Fatalf("expected lifecycle status %q, got %q", StateInstallationRequired, gotStatus)
	}
	if gotDisconnectedAt == nil {
		t.Fatal("expected disconnected_at to be set")
	}
	if clearedRepositoryCount != 0 {
		t.Fatalf("expected repositories to be cleared, got %d items", clearedRepositoryCount)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
