package githubconnection

import "time"

const (
	StateNotConnected         = "not_connected"
	StateInstallationRequired = "installation_required"
	StateConnected            = "connected"
	StateSyncing              = "syncing"
	StateSyncFailed           = "sync_failed"

	InstallationStatusAccessible           = "accessible"
	InstallationStatusPermissionMissing    = "permission_missing"
	InstallationStatusInstallationRequired = "installation_required"
	InstallationStatusSuspended            = "suspended"

	SelectionStatusNotSelected = "not_selected"
	SelectionStatusSelected    = "selected"
	SelectionStatusSyncing     = "syncing"
	SelectionStatusSyncFailed  = "sync_failed"
	SelectionStatusSynced      = "synced"
)

type StartInstallationRequest struct {
	RedirectURL *string `json:"redirectUrl"`
}

type StartInstallationResponse struct {
	InstallURL string `json:"installUrl"`
	State      string `json:"state"`
}

type ConnectionResponse struct {
	OrganizationID        string     `json:"organizationId"`
	Provider              string     `json:"provider"`
	State                 string     `json:"state"`
	InstallationID        *int64     `json:"installationId,omitempty"`
	AccountLogin          *string    `json:"accountLogin,omitempty"`
	AccountType           *string    `json:"accountType,omitempty"`
	TargetType            *string    `json:"targetType,omitempty"`
	ReconnectRequired     bool       `json:"reconnectRequired"`
	LastSyncedAt          *time.Time `json:"lastSyncedAt,omitempty"`
	LastSyncError         *string    `json:"lastSyncError,omitempty"`
	ConnectedRepositories int        `json:"connectedRepositories"`
}

type AccessibleRepositoryResponse struct {
	GithubRepositoryID int64   `json:"githubRepositoryId"`
	FullName           string  `json:"fullName"`
	Name               string  `json:"name"`
	OwnerLogin         string  `json:"ownerLogin"`
	Private            bool    `json:"private"`
	DefaultBranch      *string `json:"defaultBranch,omitempty"`
	InstallationStatus string  `json:"installationStatus"`
	SelectionStatus    string  `json:"selectionStatus"`
	LinkedRepositoryID *string `json:"linkedRepositoryId,omitempty"`
	LastSyncError      *string `json:"lastSyncError,omitempty"`
}

type ListAccessibleRepositoriesParams struct {
	OrganizationID string
	Page           int
	PageSize       int
}

type ListAccessibleRepositoriesResult struct {
	Items      []AccessibleRepositoryResponse
	TotalItems int
}

type SelectRepositoriesRequest struct {
	RepositoryIDs []int64 `json:"repositoryIds"`
	AutoSync      *bool   `json:"autoSync"`
}

type SelectRepositoriesResponse struct {
	State                 string   `json:"state"`
	SelectedRepositoryIDs []int64  `json:"selectedRepositoryIds"`
	CreatedRepositoryIDs  []string `json:"createdRepositoryIds"`
	SyncJobIDs            []string `json:"syncJobIds"`
}

type installationRecord struct {
	ID                    string
	OrganizationID        string
	InstallationID        int64
	AccountLogin          *string
	AccountType           *string
	TargetType            *string
	Status                string
	SuspendedAt           *time.Time
	LastSyncedAt          *time.Time
	LastSyncError         *string
	ConnectedRepositories int
	SyncingRepositories   int
	FailedRepositories    int
}

type accessibleRepositoryRecord struct {
	ID                 string
	InstallationDBID   string
	GithubRepositoryID int64
	Name               string
	OwnerLogin         string
	FullName           string
	Private            bool
	DefaultBranch      *string
	InstallationStatus string
	SelectionStatus    string
	LinkedRepositoryID *string
	LastSyncError      *string
}
