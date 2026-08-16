package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/auditlog"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/PangIkp/devlens/backend/internal/organization"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type OrganizationService interface {
	Create(context.Context, string, organization.CreateOrganizationRequest) (organization.OrganizationResponse, error)
	GetByID(context.Context, string) (organization.OrganizationResponse, error)
	List(context.Context, string, organization.ListParams) (organization.ListResult, error)
	Update(context.Context, string, organization.UpdateOrganizationRequest) (organization.OrganizationResponse, error)
	SoftDelete(context.Context, string) error
}

type OrganizationHandler struct {
	service    OrganizationService
	authorizer AuthorizationService
	audit      AuditLogger
}

func NewOrganizationHandler(service OrganizationService, deps ...any) *OrganizationHandler {
	authz, auditLogger := resolveHandlerDeps(deps)
	return &OrganizationHandler{service: service, authorizer: authz, audit: auditLogger}
}

func (h *OrganizationHandler) RegisterRoutes(r chi.Router) {
	if h.authorizer == nil {
		r.Post("/organizations", h.create)
		r.Get("/organizations", h.list)
		r.Get("/organizations/{organizationId}", h.get)
		r.Patch("/organizations/{organizationId}", h.update)
		r.Delete("/organizations/{organizationId}", h.softDelete)
		return
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())
		r.Post("/organizations", h.create)
		r.Get("/organizations", h.list)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleMember, authorization.RoleAdmin, authorization.RoleOwner)).Get("/organizations/{organizationId}", h.get)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Patch("/organizations/{organizationId}", h.update)
		r.With(requireOrganizationRoles(h.authorizer, authorization.RoleOwner)).Delete("/organizations/{organizationId}", h.softDelete)
	})
}

func (h *OrganizationHandler) create(w http.ResponseWriter, r *http.Request) {
	var req organization.CreateOrganizationRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	userID, ok := currentUserID(r)
	if !ok {
		if h.authorizer != nil {
			WriteError(w, r, http.StatusUnauthorized, NewUnauthorizedError("Authentication required"))
			return
		}
	}

	org, err := h.service.Create(r.Context(), userID, req)
	if err != nil {
		writeOrganizationError(w, r, err)
		return
	}

	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(org.ID),
		ActorUserID:    stringRef(userID),
		Action:         "organization.create",
		ResourceType:   "organization",
		ResourceID:     stringRef(org.ID),
		Metadata: map[string]any{
			"githubId": org.GithubID,
			"slug":     org.Slug,
			"name":     org.Name,
		},
	})
	WriteData(w, http.StatusCreated, org)
}

func (h *OrganizationHandler) list(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	userID, ok := currentUserID(r)
	if !ok {
		if h.authorizer != nil {
			WriteError(w, r, http.StatusUnauthorized, NewUnauthorizedError("Authentication required"))
			return
		}
	}

	result, err := h.service.List(r.Context(), userID, organization.ListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
	})
	if err != nil {
		writeOrganizationError(w, r, err)
		return
	}

	WritePage(w, http.StatusOK, result.Items, NewPaginationMeta(page.Page, page.PageSize, result.TotalItems))
}

func (h *OrganizationHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "organizationId")
	if err := validateOrganizationID(id); err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	org, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeOrganizationError(w, r, err)
		return
	}

	WriteData(w, http.StatusOK, org)
}

func (h *OrganizationHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "organizationId")
	if err := validateOrganizationID(id); err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req organization.UpdateOrganizationRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	org, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		writeOrganizationError(w, r, err)
		return
	}

	userID, _ := currentUserID(r)
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(id),
		ActorUserID:    stringRef(userID),
		Action:         "organization.update",
		ResourceType:   "organization",
		ResourceID:     stringRef(id),
		Metadata: map[string]any{
			"slug": org.Slug,
			"name": org.Name,
		},
	})
	WriteData(w, http.StatusOK, org)
}

func (h *OrganizationHandler) softDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "organizationId")
	if err := validateOrganizationID(id); err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	if err := h.service.SoftDelete(r.Context(), id); err != nil {
		writeOrganizationError(w, r, err)
		return
	}

	userID, _ := currentUserID(r)
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		OrganizationID: stringRef(id),
		ActorUserID:    stringRef(userID),
		Action:         "organization.delete",
		ResourceType:   "organization",
		ResourceID:     stringRef(id),
	})
	w.WriteHeader(http.StatusNoContent)
}

func writeOrganizationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, organization.ErrOrganizationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization not found"))
	case errors.Is(err, organization.ErrOrganizationConflict):
		WriteError(w, r, http.StatusConflict, NewConflictError("Organization already exists"))
	default:
		var validationErr *organization.ValidationError
		if errors.As(err, &validationErr) {
			details := make([]ValidationIssue, 0, len(validationErr.Details))
			for _, issue := range validationErr.Details {
				details = append(details, ValidationIssue{
					Field:   issue.Field,
					Message: issue.Message,
				})
			}

			WriteError(w, r, http.StatusBadRequest, NewValidationError(validationErr.Message, details...))
			return
		}

		WriteError(w, r, http.StatusInternalServerError, NewInternalError())
	}
}

func isValidUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil
}

func validateOrganizationID(value string) *Error {
	if isValidUUID(value) {
		return nil
	}

	err := NewValidationError(
		"request validation failed",
		FieldInvalid("organizationId", "must be a valid UUID"),
	)
	return &err
}
