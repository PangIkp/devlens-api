package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/PangIkp/devlens/backend/internal/githubconnection"
	"github.com/go-chi/chi/v5"
)

type GitHubConnectionService interface {
	GetConnection(context.Context, string) (githubconnection.ConnectionResponse, error)
	StartInstallation(context.Context, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error)
	CompleteInstallation(context.Context, string, int64) (githubconnection.ConnectionResponse, error)
	ListAccessibleRepositories(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error)
	SelectRepositories(context.Context, string, githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error)
}

type GitHubConnectionHandler struct {
	service GitHubConnectionService
}

func NewGitHubConnectionHandler(service GitHubConnectionService) *GitHubConnectionHandler {
	return &GitHubConnectionHandler{service: service}
}

func (h *GitHubConnectionHandler) RegisterRoutes(r chi.Router) {
	r.Get("/organizations/{organizationId}/github/connection", h.getConnection)
	r.Post("/organizations/{organizationId}/github/installations/start", h.startInstallation)
	r.Get("/organizations/{organizationId}/github/installations/callback", h.completeInstallation)
	r.Get("/organizations/{organizationId}/github/repositories", h.listAccessibleRepositories)
	r.Post("/organizations/{organizationId}/github/repositories/select", h.selectRepositories)
}

func (h *GitHubConnectionHandler) getConnection(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	item, svcErr := h.service.GetConnection(r.Context(), organizationID)
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusOK, item)
}

func (h *GitHubConnectionHandler) startInstallation(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req githubconnection.StartInstallationRequest
	if r.ContentLength > 0 {
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, DecodeError(err))
			return
		}
	}

	item, svcErr := h.service.StartInstallation(r.Context(), organizationID, req)
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusOK, item)
}

func (h *GitHubConnectionHandler) completeInstallation(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	installationID, parseErr := parsePositiveInt64(r.URL.Query().Get("installation_id"), "installation_id")
	if parseErr != nil {
		WriteError(w, r, http.StatusBadRequest, *parseErr)
		return
	}

	item, svcErr := h.service.CompleteInstallation(r.Context(), organizationID, installationID)
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusOK, item)
}

func (h *GitHubConnectionHandler) listAccessibleRepositories(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	page, parseErr := ParsePagination(r)
	if parseErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(parseErr))
		return
	}

	result, svcErr := h.service.ListAccessibleRepositories(r.Context(), githubconnection.ListAccessibleRepositoriesParams{
		OrganizationID: organizationID,
		Page:           page.Page,
		PageSize:       page.PageSize,
	})
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}

	WritePage(w, http.StatusOK, result.Items, NewPaginationMeta(page.Page, page.PageSize, result.TotalItems))
}

func (h *GitHubConnectionHandler) selectRepositories(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req githubconnection.SelectRepositoriesRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	item, svcErr := h.service.SelectRepositories(r.Context(), organizationID, req)
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusAccepted, item)
}

func writeGitHubConnectionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, githubconnection.ErrOrganizationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization not found"))
	case errors.Is(err, githubconnection.ErrInstallationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("GitHub installation not found"))
	case errors.Is(err, githubconnection.ErrAccessibleRepositoryNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Accessible GitHub repository not found"))
	case errors.Is(err, githubconnection.ErrConnectionNotConfigured):
		WriteError(w, r, http.StatusConflict, NewConflictError("GitHub App is not configured"))
	default:
		var validationErr *githubconnection.ValidationError
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

func parsePositiveInt64(raw string, field string) (int64, *Error) {
	if raw == "" {
		err := NewValidationError("request validation failed", FieldInvalid(field, "is required"))
		return 0, &err
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		e := NewValidationError("request validation failed", FieldInvalid(field, "must be an integer"))
		return 0, &e
	}
	if value < 1 {
		e := NewValidationError("request validation failed", FieldInvalid(field, "must be greater than or equal to 1"))
		return 0, &e
	}
	return value, nil
}
