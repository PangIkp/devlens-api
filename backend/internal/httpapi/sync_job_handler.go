package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/go-chi/chi/v5"
)

type SyncJobService interface {
	Create(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error)
	GetByID(context.Context, string) (syncjob.SyncJobResponse, error)
	ListByRepository(context.Context, syncjob.ListParams) (syncjob.ListResult, error)
}

type SyncJobHandler struct {
	service SyncJobService
}

func NewSyncJobHandler(service SyncJobService) *SyncJobHandler {
	return &SyncJobHandler{service: service}
}

func (h *SyncJobHandler) RegisterRoutes(r chi.Router) {
	r.Post("/repositories/{repositoryId}/sync", h.create)
	r.Get("/repositories/{repositoryId}/sync-jobs", h.list)
	r.Get("/sync-jobs/{syncJobId}", h.get)
}

func (h *SyncJobHandler) create(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	var req syncjob.CreateSyncRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, DecodeError(err))
			return
		}
	}

	item, svcErr := h.service.Create(r.Context(), repositoryID, req)
	if svcErr != nil {
		writeSyncJobError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusAccepted, item)
}

func (h *SyncJobHandler) list(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	page, parseErr := ParsePagination(r)
	if parseErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(parseErr))
		return
	}

	result, svcErr := h.service.ListByRepository(r.Context(), syncjob.ListParams{
		RepositoryID: repositoryID,
		Page:         page.Page,
		PageSize:     page.PageSize,
		Status:       strings.TrimSpace(r.URL.Query().Get("status")),
		SortOrder:    strings.TrimSpace(r.URL.Query().Get("sortOrder")),
	})
	if svcErr != nil {
		writeSyncJobError(w, r, svcErr)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Data []syncjob.SyncJobResponse `json:"data"`
		Meta PaginationMeta            `json:"meta"`
	}{
		Data: result.Items,
		Meta: NewPaginationMeta(page.Page, page.PageSize, result.TotalItems),
	})
}

func (h *SyncJobHandler) get(w http.ResponseWriter, r *http.Request) {
	syncJobID, err := validateUUIDPathParam("syncJobId", chi.URLParam(r, "syncJobId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	item, svcErr := h.service.GetByID(r.Context(), syncJobID)
	if svcErr != nil {
		writeSyncJobError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusOK, item)
}

func writeSyncJobError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, syncjob.ErrRepositoryNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Repository not found"))
	case errors.Is(err, syncjob.ErrSyncJobNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Sync job not found"))
	case errors.Is(err, syncjob.ErrSyncJobConflict):
		WriteError(w, r, http.StatusConflict, NewConflictError("A sync job is already running for this repository"))
	default:
		var validationErr *syncjob.ValidationError
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
