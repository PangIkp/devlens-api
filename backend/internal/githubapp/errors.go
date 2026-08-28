package githubapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAppCredentialsInvalid = errors.New("github app credentials are invalid")
	ErrInstallationNotFound  = errors.New("github app installation was not found")
)

type APIError struct {
	StatusCode int
	Message    string
	kind       error
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = httpStatusMessage(e.StatusCode)
	}
	return fmt.Sprintf("github app request failed: status=%d message=%s", e.StatusCode, message)
}

func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.kind
}

func (e *APIError) Is(target error) bool {
	if e == nil {
		return false
	}
	if target == ErrAppCredentialsInvalid {
		return e.StatusCode == 401
	}
	if target == ErrInstallationNotFound {
		return e.kind == ErrInstallationNotFound
	}
	return false
}

func newAPIError(statusCode int, path string, body []byte) error {
	apiErr := &APIError{
		StatusCode: statusCode,
		Message:    githubAPIMessage(body),
	}
	switch {
	case statusCode == 401:
		apiErr.kind = ErrAppCredentialsInvalid
	case statusCode == 404 && strings.HasPrefix(path, "/app/installations/"):
		apiErr.kind = ErrInstallationNotFound
	}
	return apiErr
}

func githubAPIMessage(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		return strings.TrimSpace(payload.Message)
	}
	return httpStatusMessage(0)
}

func httpStatusMessage(statusCode int) string {
	switch statusCode {
	case 400:
		return "Bad Request"
	case 401:
		return "Bad credentials"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	default:
		return "GitHub API request failed"
	}
}
