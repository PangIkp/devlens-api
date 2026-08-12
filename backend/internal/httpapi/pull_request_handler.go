package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/pullrequest"
	"github.com/go-chi/chi/v5"
)

type PullRequestService interface {
	List(context.Context, pullrequest.ListParams) (pullrequest.ListResult, error)
	GetByID(context.Context, string) (pullrequest.Response, error)
}

type PullRequestHandler struct {
	service PullRequestService
}

func NewPullRequestHandler(service PullRequestService) *PullRequestHandler {
	return &PullRequestHandler{service: service}
}

func (h *PullRequestHandler) RegisterRoutes(r chi.Router) {
	r.Get("/pull-requests", h.list)
	r.Get("/pull-requests/{pullRequestId}", h.get)
}

func (h *PullRequestHandler) list(w http.ResponseWriter, r *http.Request) {
	page, parseErr := ParsePagination(r)
	if parseErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(parseErr))
		return
	}

	result, svcErr := h.service.List(r.Context(), pullrequest.ListParams{
		RepositoryID: strings.TrimSpace(r.URL.Query().Get("repositoryId")),
		Page:         page.Page,
		PageSize:     page.PageSize,
		Status:       strings.TrimSpace(r.URL.Query().Get("status")),
		Search:       strings.TrimSpace(r.URL.Query().Get("search")),
		SortBy:       strings.TrimSpace(r.URL.Query().Get("sortBy")),
		SortOrder:    strings.TrimSpace(r.URL.Query().Get("sortOrder")),
	})
	if svcErr != nil {
		writePullRequestError(w, r, svcErr)
		return
	}

	WritePage(w, http.StatusOK, result.Items, NewPaginationMeta(page.Page, page.PageSize, result.TotalItems))
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
	case errors.Is(err, pullrequest.ErrRepositoryNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Repository not found"))
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
