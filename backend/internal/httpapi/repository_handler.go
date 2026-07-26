package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	devrepository "github.com/PangIkp/devlens/backend/internal/repository"
	"github.com/go-chi/chi/v5"
)

type RepositoryService interface {
	Create(context.Context, string, devrepository.CreateRepositoryRequest) (devrepository.RepositoryResponse, error)
	GetByID(context.Context, string) (devrepository.RepositoryResponse, error)
	List(context.Context, devrepository.ListParams) (devrepository.ListResult, error)
	Update(context.Context, string, devrepository.UpdateRepositoryRequest) (devrepository.RepositoryResponse, error)
}

type RepositoryHandler struct {
	service RepositoryService
}

func NewRepositoryHandler(service RepositoryService) *RepositoryHandler {
	return &RepositoryHandler{service: service}
}

func (h *RepositoryHandler) RegisterRoutes(r chi.Router) {
	r.Post("/organizations/{organizationId}/repositories", h.create)
	r.Get("/organizations/{organizationId}/repositories", h.list)
	r.Get("/repositories/{repositoryId}", h.get)
	r.Patch("/repositories/{repositoryId}", h.update)
}

func (h *RepositoryHandler) create(w http.ResponseWriter, r *http.Request) {
	organizationID, err := validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req devrepository.CreateRepositoryRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	item, svcErr := h.service.Create(r.Context(), organizationID, req)
	if svcErr != nil {
		writeRepositoryError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusCreated, item)
}

func (h *RepositoryHandler) list(w http.ResponseWriter, r *http.Request) {
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

	params := devrepository.ListParams{
		OrganizationID: organizationID,
		Page:           page.Page,
		PageSize:       page.PageSize,
		Status:         strings.TrimSpace(r.URL.Query().Get("status")),
		Search:         strings.TrimSpace(r.URL.Query().Get("search")),
		SortBy:         strings.TrimSpace(r.URL.Query().Get("sortBy")),
		SortOrder:      strings.TrimSpace(r.URL.Query().Get("sortOrder")),
	}

	result, svcErr := h.service.List(r.Context(), params)
	if svcErr != nil {
		writeRepositoryError(w, r, svcErr)
		return
	}

	WritePage(w, http.StatusOK, result.Items, NewPaginationMeta(page.Page, page.PageSize, result.TotalItems))
}

func (h *RepositoryHandler) get(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	item, svcErr := h.service.GetByID(r.Context(), repositoryID)
	if svcErr != nil {
		writeRepositoryError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusOK, item)
}

func (h *RepositoryHandler) update(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req devrepository.UpdateRepositoryRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	item, svcErr := h.service.Update(r.Context(), repositoryID, req)
	if svcErr != nil {
		writeRepositoryError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusOK, item)
}

func writeRepositoryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, devrepository.ErrOrganizationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization not found"))
	case errors.Is(err, devrepository.ErrRepositoryNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Repository not found"))
	case errors.Is(err, devrepository.ErrRepositoryConflict):
		WriteError(w, r, http.StatusConflict, NewConflictError("Repository already exists"))
	default:
		var validationErr *devrepository.ValidationError
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
