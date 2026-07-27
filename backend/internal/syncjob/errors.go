package syncjob

import "errors"

var (
	ErrRepositoryNotFound = errors.New("repository not found")
	ErrSyncJobNotFound    = errors.New("sync job not found")
	ErrSyncJobConflict    = errors.New("sync job conflict")
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
