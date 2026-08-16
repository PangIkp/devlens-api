package syncjob

import "errors"

var (
	ErrRepositoryNotFound     = errors.New("repository not found")
	ErrRepositoryNotConnected = errors.New("repository github installation is not connected")
	ErrRepositoryNotSelected  = errors.New("repository github installation selection is required")
	ErrRepositoryDeactivated  = errors.New("repository is deactivated")
	ErrSyncJobNotFound        = errors.New("sync job not found")
	ErrSyncJobConflict        = errors.New("sync job conflict")
	ErrSyncJobRetryState      = errors.New("sync job retry not allowed")
	ErrSyncJobCancelState     = errors.New("sync job cancel not allowed")
)

type ValidationError struct {
	Message string
	Details []ValidationIssue
}

func (e *ValidationError) Error() string {
	return e.Message
}

type ValidationIssue struct {
	Field   string
	Message string
}
