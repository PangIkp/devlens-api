package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/auditlog"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/go-chi/chi/v5"
)

type SyncJobService interface {
	Create(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error)
	Enqueue(context.Context, string, syncjob.CreateSyncRequest) (syncjob.SyncJobResponse, error)
	GetByID(context.Context, string) (syncjob.SyncJobResponse, error)
	ListByRepository(context.Context, syncjob.ListParams) (syncjob.ListResult, error)
	Retry(context.Context, string) (syncjob.SyncJobResponse, error)
	Cancel(context.Context, string) (syncjob.SyncJobResponse, error)
}

type SyncJobHandler struct {
	service    SyncJobService
	authorizer AuthorizationService
	audit      AuditLogger
}

func NewSyncJobHandler(service SyncJobService, deps ...any) *SyncJobHandler {
	authz, auditLogger := resolveHandlerDeps(deps)
	return &SyncJobHandler{service: service, authorizer: authz, audit: auditLogger}
}

func (h *SyncJobHandler) RegisterRoutes(r chi.Router) {
	if h.authorizer == nil {
		r.Post("/repositories/{repositoryId}/sync", h.create)
		r.Get("/repositories/{repositoryId}/sync-jobs", h.list)
		r.Get("/sync-jobs/{syncJobId}", h.get)
		r.Post("/sync-jobs/{syncJobId}/retry", h.retry)
		r.Post("/sync-jobs/{syncJobId}/cancel", h.cancel)
		return
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())
		r.With(requireRepositoryPathRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Post("/repositories/{repositoryId}/sync", h.create)
		r.With(requireRepositoryPathRoles(h.authorizer, authorization.RoleMember, authorization.RoleAdmin, authorization.RoleOwner)).Get("/repositories/{repositoryId}/sync-jobs", h.list)
		r.With(requireSyncJobRoles(h.authorizer, authorization.RoleMember, authorization.RoleAdmin, authorization.RoleOwner)).Get("/sync-jobs/{syncJobId}", h.get)
		r.With(requireSyncJobRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Post("/sync-jobs/{syncJobId}/retry", h.retry)
		r.With(requireSyncJobRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Post("/sync-jobs/{syncJobId}/cancel", h.cancel)
	})
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
	req.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))

	item, svcErr := h.service.Enqueue(r.Context(), repositoryID, req)
	if svcErr != nil {
		writeSyncJobError(w, r, svcErr)
		return
	}
	userID, _ := currentUserID(r)
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		ActorUserID:  stringRef(userID),
		Action:       "sync_job.create",
		ResourceType: "sync_job",
		ResourceID:   stringRef(item.ID),
		Metadata: map[string]any{
			"repositoryId":   item.RepositoryID,
			"status":         item.Status,
			"idempotencyKey": req.IdempotencyKey,
		},
	})

	WriteData(w, http.StatusAccepted, item)
}

func (h *SyncJobHandler) retry(w http.ResponseWriter, r *http.Request) {
	syncJobID, err := validateUUIDPathParam("syncJobId", chi.URLParam(r, "syncJobId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	item, svcErr := h.service.Retry(r.Context(), syncJobID)
	if svcErr != nil {
		writeSyncJobError(w, r, svcErr)
		return
	}
	userID, _ := currentUserID(r)
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		ActorUserID:  stringRef(userID),
		Action:       "sync_job.retry",
		ResourceType: "sync_job",
		ResourceID:   stringRef(item.ID),
		Metadata: map[string]any{
			"repositoryId": item.RepositoryID,
			"status":       item.Status,
		},
	})

	WriteData(w, http.StatusAccepted, item)
}

func (h *SyncJobHandler) cancel(w http.ResponseWriter, r *http.Request) {
	syncJobID, err := validateUUIDPathParam("syncJobId", chi.URLParam(r, "syncJobId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	item, svcErr := h.service.Cancel(r.Context(), syncJobID)
	if svcErr != nil {
		writeSyncJobError(w, r, svcErr)
		return
	}
	userID, _ := currentUserID(r)
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		ActorUserID:  stringRef(userID),
		Action:       "sync_job.cancel",
		ResourceType: "sync_job",
		ResourceID:   stringRef(item.ID),
		Metadata: map[string]any{
			"repositoryId": item.RepositoryID,
			"status":       item.Status,
		},
	})

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
	case errors.Is(err, syncjob.ErrSyncJobRetryState):
		WriteError(w, r, http.StatusConflict, NewConflictError("Only failed sync jobs can be retried"))
	case errors.Is(err, syncjob.ErrSyncJobCancelState):
		WriteError(w, r, http.StatusConflict, NewConflictError("Only pending or running sync jobs can be canceled"))
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
