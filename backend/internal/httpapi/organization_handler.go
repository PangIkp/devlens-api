package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/organization"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type OrganizationService interface {
	Create(context.Context, organization.CreateOrganizationRequest) (organization.OrganizationResponse, error)
	GetByID(context.Context, string) (organization.OrganizationResponse, error)
	List(context.Context, organization.ListParams) (organization.ListResult, error)
}

type OrganizationHandler struct {
	service OrganizationService
}

func NewOrganizationHandler(service OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{service: service}
}

func (h *OrganizationHandler) RegisterRoutes(r chi.Router) {
	r.Post("/organizations", h.create)
	r.Get("/organizations", h.list)
	r.Get("/organizations/{organizationId}", h.get)
}

func (h *OrganizationHandler) create(w http.ResponseWriter, r *http.Request) {
	var req organization.CreateOrganizationRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	org, err := h.service.Create(r.Context(), req)
	if err != nil {
		writeOrganizationError(w, r, err)
		return
	}

	WriteData(w, http.StatusCreated, org)
}

func (h *OrganizationHandler) list(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePagination(r)
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	result, err := h.service.List(r.Context(), organization.ListParams{
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
	if !isValidUUID(id) {
		WriteError(w, r, http.StatusBadRequest, NewValidationError(
			"request validation failed",
			FieldInvalid("organizationId", "must be a valid UUID"),
		))
		return
	}

	org, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		writeOrganizationError(w, r, err)
		return
	}

	WriteData(w, http.StatusOK, org)
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
