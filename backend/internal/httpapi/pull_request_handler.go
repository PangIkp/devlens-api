package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/pullrequest"
	"github.com/go-chi/chi/v5"
)

type PullRequestService interface {
	GetByID(context.Context, string) (pullrequest.Response, error)
}

type PullRequestHandler struct {
	service PullRequestService
}

func NewPullRequestHandler(service PullRequestService) *PullRequestHandler {
	return &PullRequestHandler{service: service}
}

func (h *PullRequestHandler) RegisterRoutes(r chi.Router) {
	r.Get("/pull-requests/{pullRequestId}", h.get)
}

func (h *PullRequestHandler) get(w http.ResponseWriter, r *http.Request) {
	pullRequestID, err := validateUUIDPathParam("pullRequestId", chi.URLParam(r, "pullRequestId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	item, svcErr := h.service.GetByID(r.Context(), pullRequestID)
	if svcErr != nil {
		writePullRequestError(w, r, svcErr)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func writePullRequestError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pullrequest.ErrPullRequestNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Pull request not found"))
	default:
		var validationErr *pullrequest.ValidationError
		if errors.As(err, &validationErr) {
			details := make([]ValidationIssue, 0, len(validationErr.Details))
			for _, issue := range validationErr.Details {
				details = append(details, ValidationIssue{Field: issue.Field, Message: issue.Message})
			}
			WriteError(w, r, http.StatusBadRequest, NewValidationError(validationErr.Message, details...))
			return
		}
		WriteError(w, r, http.StatusInternalServerError, NewInternalError())
	}
}
