package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/organizationmember"
	"github.com/go-chi/chi/v5"
)

type OrganizationMemberService interface {
	List(context.Context, string) ([]organizationmember.MemberResponse, error)
	Create(context.Context, string, organizationmember.CreateOrganizationMemberRequest) (organizationmember.MemberResponse, error)
	Update(context.Context, string, string, organizationmember.UpdateOrganizationMemberRequest) (organizationmember.MemberResponse, error)
	Delete(context.Context, string, string) error
}

type OrganizationMemberHandler struct {
	service OrganizationMemberService
}

func NewOrganizationMemberHandler(service OrganizationMemberService) *OrganizationMemberHandler {
	return &OrganizationMemberHandler{service: service}
}

func (h *OrganizationMemberHandler) RegisterRoutes(r chi.Router) {
	r.Get("/organizations/{organizationId}/members", h.list)
	r.Post("/organizations/{organizationId}/members", h.create)
	r.Patch("/organizations/{organizationId}/members/{memberId}", h.update)
	r.Delete("/organizations/{organizationId}/members/{memberId}", h.delete)
}

func (h *OrganizationMemberHandler) list(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}
	items, svcErr := h.service.List(r.Context(), organizationID)
	if svcErr != nil {
		writeOrganizationMemberError(w, r, svcErr)
		return
	}
	WriteData(w, http.StatusOK, items)
}

func (h *OrganizationMemberHandler) create(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req organizationmember.CreateOrganizationMemberRequest
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(decodeErr))
		return
	}
	if _, userErr := validateUUIDPathParam("userId", req.UserID); userErr != nil {
		WriteError(w, r, http.StatusBadRequest, *userErr)
		return
	}

	item, svcErr := h.service.Create(r.Context(), organizationID, req)
	if svcErr != nil {
		writeOrganizationMemberError(w, r, svcErr)
		return
	}
	WriteData(w, http.StatusCreated, item)
}

func (h *OrganizationMemberHandler) update(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}
	memberID, err := validateUUIDPathParam("memberId", chi.URLParam(r, "memberId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req organizationmember.UpdateOrganizationMemberRequest
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(decodeErr))
		return
	}

	item, svcErr := h.service.Update(r.Context(), organizationID, memberID, req)
	if svcErr != nil {
		writeOrganizationMemberError(w, r, svcErr)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func (h *OrganizationMemberHandler) delete(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}
	memberID, err := validateUUIDPathParam("memberId", chi.URLParam(r, "memberId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	if svcErr := h.service.Delete(r.Context(), organizationID, memberID); svcErr != nil {
		writeOrganizationMemberError(w, r, svcErr)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeOrganizationMemberError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, organizationmember.ErrOrganizationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization not found"))
	case errors.Is(err, organizationmember.ErrMemberNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization member not found"))
	case errors.Is(err, organizationmember.ErrMemberConflict):
		WriteError(w, r, http.StatusConflict, NewConflictError("Organization member already exists"))
	case errors.Is(err, organizationmember.ErrLastOwnerConflict):
		WriteError(w, r, http.StatusConflict, NewConflictError("Cannot remove the last owner"))
	case errors.Is(err, organizationmember.ErrUserNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("User not found"))
	default:
		var validationErr *organizationmember.ValidationError
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

func validateUUIDPathParam(field string, value string) (string, *Error) {
	if isValidUUID(value) {
		return value, nil
	}
	err := NewValidationError("request validation failed", FieldInvalid(field, "must be a valid UUID"))
	return "", &err
}
