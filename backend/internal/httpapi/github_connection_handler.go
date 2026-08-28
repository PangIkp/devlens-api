package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/auditlog"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/githubconnection"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/go-chi/chi/v5"
)

type GitHubConnectionService interface {
	GetConnection(context.Context, string) (githubconnection.ConnectionResponse, error)
	StartInstallation(context.Context, string, string, githubconnection.StartInstallationRequest) (githubconnection.StartInstallationResponse, error)
	CompleteInstallation(context.Context, string, int64, string, string) (githubconnection.ConnectionResponse, error)
	ListAccessibleRepositories(context.Context, githubconnection.ListAccessibleRepositoriesParams) (githubconnection.ListAccessibleRepositoriesResult, error)
	SelectRepositories(context.Context, string, githubconnection.SelectRepositoriesRequest) (githubconnection.SelectRepositoriesResponse, error)
	Disconnect(context.Context, string, string) error
}

const errorCodeGitHubInstallationAlreadyLinked = "GITHUB_INSTALLATION_ALREADY_LINKED"

type GitHubConnectionHandler struct {
	service    GitHubConnectionService
	authorizer AuthorizationService
	audit      AuditLogger
}

func NewGitHubConnectionHandler(service GitHubConnectionService, deps ...any) *GitHubConnectionHandler {
	authz, auditLogger := resolveHandlerDeps(deps)
	return &GitHubConnectionHandler{service: service, authorizer: authz, audit: auditLogger}
}

func (h *GitHubConnectionHandler) RegisterRoutes(r chi.Router) {
	if h.authorizer == nil {
		r.Get("/organizations/{organizationId}/github/connection", h.getConnection)
		r.Delete("/organizations/{organizationId}/github/connection", h.disconnect)
		r.Post("/organizations/{organizationId}/github/installations/start", h.startInstallation)
		r.Get("/organizations/{organizationId}/github/installations/callback", h.completeInstallation)
		r.Get("/organizations/{organizationId}/github/repositories", h.listAccessibleRepositories)
		r.Post("/organizations/{organizationId}/github/repositories/select", h.selectRepositories)
		return
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleMember, authorization.RoleAdmin, authorization.RoleOwner)).Get("/organizations/{organizationId}/github/connection", h.getConnection)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Delete("/organizations/{organizationId}/github/connection", h.disconnect)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Post("/organizations/{organizationId}/github/installations/start", h.startInstallation)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Get("/organizations/{organizationId}/github/installations/callback", h.completeInstallation)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Get("/organizations/{organizationId}/github/repositories", h.listAccessibleRepositories)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Post("/organizations/{organizationId}/github/repositories/select", h.selectRepositories)
	})
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

	WriteDataConditional(w, r, http.StatusOK, item)
}

func (h *GitHubConnectionHandler) disconnect(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	userID, _ := currentUserID(r)
	if svcErr := h.service.Disconnect(r.Context(), organizationID, userID); svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}

	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(organizationID),
		ActorUserID:    stringRef(userID),
		Action:         "github_connection.disconnect",
		ResourceType:   "github_installation",
		ResourceID:     stringRef(organizationID),
	})

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

	userID, _ := currentUserID(r)
	item, svcErr := h.service.StartInstallation(r.Context(), organizationID, userID, req)
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(organizationID),
		ActorUserID:    stringRef(userID),
		Action:         "github_connection.installation_start",
		ResourceType:   "github_installation",
		Metadata: map[string]any{
			"installUrl": item.InstallURL,
			"state":      item.State,
		},
	})

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
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		WriteError(w, r, http.StatusBadRequest, NewValidationError("request validation failed", FieldInvalid("state", "is required")))
		return
	}

	userID, _ := currentUserID(r)
	item, svcErr := h.service.CompleteInstallation(r.Context(), organizationID, installationID, state, userID)
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(organizationID),
		ActorUserID:    stringRef(userID),
		Action:         "github_connection.installation_complete",
		ResourceType:   "github_installation",
		ResourceID:     stringRef(organizationID),
		Metadata: map[string]any{
			"installationId": installationID,
			"state":          item.State,
			"accountLogin":   item.AccountLogin,
		},
	})

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
		Search:         strings.TrimSpace(r.URL.Query().Get("search")),
	})
	if svcErr != nil {
		writeGitHubConnectionError(w, r, svcErr)
		return
	}

	WritePageConditional(w, r, http.StatusOK, result.Items, NewPaginationMeta(page.Page, page.PageSize, result.TotalItems))
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
	userID, _ := currentUserID(r)
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(organizationID),
		ActorUserID:    stringRef(userID),
		Action:         "github_connection.repositories_select",
		ResourceType:   "github_installation",
		ResourceID:     stringRef(organizationID),
		Metadata: map[string]any{
			"selectedRepositoryIds": item.SelectedRepositoryIDs,
			"createdRepositoryIds":  item.CreatedRepositoryIDs,
			"syncJobIds":            item.SyncJobIDs,
			"state":                 item.State,
		},
	})

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
	case errors.Is(err, githubconnection.ErrInstallationLinkedToAnotherOrg):
		WriteError(w, r, http.StatusConflict, NewConflictErrorWithCode(errorCodeGitHubInstallationAlreadyLinked, "This GitHub installation is already connected to a different organization. Disconnect it there first, or install the app on a different GitHub account or organization."))
	case errors.Is(err, githubconnection.ErrInstallationLinkedToAnotherUser):
		WriteError(w, r, http.StatusConflict, NewConflictErrorWithCode(errorCodeGitHubInstallationAlreadyLinked, "This GitHub installation is already connected to a different user. Disconnect it there first, or install the app on a different GitHub account or organization."))
	case errors.Is(err, githubconnection.ErrInvalidInstallationState):
		WriteError(w, r, http.StatusBadRequest, NewValidationError("request validation failed", FieldInvalid("state", "is invalid or expired")))
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
