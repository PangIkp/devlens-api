package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/auditlog"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/PangIkp/devlens/backend/internal/orgretention"
	"github.com/go-chi/chi/v5"
)

type OrganizationRetentionSettingsService interface {
	Get(context.Context, string) (orgretention.Response, error)
	Update(context.Context, string, orgretention.UpdateRequest, *string) (orgretention.Response, error)
}

type OrganizationRetentionSettingsHandler struct {
	service    OrganizationRetentionSettingsService
	authorizer AuthorizationService
	audit      AuditLogger
}

func NewOrganizationRetentionSettingsHandler(service OrganizationRetentionSettingsService, deps ...any) *OrganizationRetentionSettingsHandler {
	authz, auditLogger := resolveHandlerDeps(deps)
	return &OrganizationRetentionSettingsHandler{service: service, authorizer: authz, audit: auditLogger}
}

func (h *OrganizationRetentionSettingsHandler) RegisterRoutes(r chi.Router) {
	if h.authorizer == nil {
		r.Get("/organizations/{organizationId}/settings/retention", h.get)
		r.Put("/organizations/{organizationId}/settings/retention", h.update)
		return
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleMember, authorization.RoleAdmin, authorization.RoleOwner)).Get("/organizations/{organizationId}/settings/retention", h.get)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Put("/organizations/{organizationId}/settings/retention", h.update)
	})
}

func (h *OrganizationRetentionSettingsHandler) get(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	item, svcErr := h.service.Get(r.Context(), organizationID)
	if svcErr != nil {
		writeOrganizationRetentionSettingsError(w, r, svcErr)
		return
	}

	WriteDataConditional(w, r, http.StatusOK, item)
}

func (h *OrganizationRetentionSettingsHandler) update(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req orgretention.UpdateRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	userID, _ := currentUserID(r)
	var actorID *string
	if userID != "" {
		actorID = &userID
	}

	item, svcErr := h.service.Update(r.Context(), organizationID, req, actorID)
	if svcErr != nil {
		writeOrganizationRetentionSettingsError(w, r, svcErr)
		return
	}

	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(organizationID),
		ActorUserID:    stringRef(userID),
		Action:         "organization_retention_settings.update",
		ResourceType:   "organization_retention_settings",
		ResourceID:     stringRef(organizationID),
	})

	WriteData(w, http.StatusOK, item)
}

func writeOrganizationRetentionSettingsError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgretention.ErrOrganizationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization not found"))
	default:
		var validationErr *orgretention.ValidationError
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
