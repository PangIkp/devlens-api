package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/insights"
	"github.com/go-chi/chi/v5"
)

type InsightService interface {
	List(context.Context, insights.ListParams) (insights.ListResult, error)
	Review(context.Context, string, string, insights.ReviewRequest) (insights.StatusResult, error)
	Dismiss(context.Context, string, string) (insights.StatusResult, error)
	Reopen(context.Context, string, string) (insights.StatusResult, error)
}

type InsightHandler struct {
	service InsightService
}

func NewInsightHandler(service InsightService) *InsightHandler {
	return &InsightHandler{service: service}
}

func (h *InsightHandler) RegisterRoutes(r chi.Router) {
	r.Get("/insights", h.list)
	r.Get("/organizations/{organizationId}/insights", h.list)
	r.Post("/organizations/{organizationId}/insights/{insightKey}/review", h.review)
	r.Post("/organizations/{organizationId}/insights/{insightKey}/dismiss", h.dismiss)
	r.Post("/organizations/{organizationId}/insights/{insightKey}/reopen", h.reopen)
}

func (h *InsightHandler) list(w http.ResponseWriter, r *http.Request) {
	organizationID, err := h.resolveOrganizationID(r)
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}
	page, pageErr := ParsePagination(r)
	if pageErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(pageErr))
		return
	}
	bounds, queryErr := parseDateRangeQuery(r)
	if queryErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(queryErr))
		return
	}

	params := insights.ListParams{
		OrganizationID: organizationID,
		RepositoryID:   strings.TrimSpace(r.URL.Query().Get("repositoryId")),
		Type:           strings.TrimSpace(r.URL.Query().Get("type")),
		Status:         strings.TrimSpace(r.URL.Query().Get("status")),
		From:           bounds.From,
		To:             bounds.To,
		Page:           page.Page,
		PageSize:       page.PageSize,
	}

	result, svcErr := h.service.List(r.Context(), params)
	if svcErr != nil {
		writeInsightError(w, r, svcErr)
		return
	}
	WritePage(w, http.StatusOK, result.Items, NewPaginationMeta(page.Page, page.PageSize, result.TotalItems))
}

func (h *InsightHandler) review(w http.ResponseWriter, r *http.Request) {
	organizationID, insightKey, ok := h.parseActionPath(w, r)
	if !ok {
		return
	}

	var req insights.ReviewRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := DecodeJSON(r, &req); err != nil {
			WriteError(w, r, http.StatusBadRequest, DecodeError(err))
			return
		}
	}

	item, err := h.service.Review(r.Context(), organizationID, insightKey, req)
	if err != nil {
		writeInsightError(w, r, err)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func (h *InsightHandler) dismiss(w http.ResponseWriter, r *http.Request) {
	organizationID, insightKey, ok := h.parseActionPath(w, r)
	if !ok {
		return
	}

	item, err := h.service.Dismiss(r.Context(), organizationID, insightKey)
	if err != nil {
		writeInsightError(w, r, err)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func (h *InsightHandler) reopen(w http.ResponseWriter, r *http.Request) {
	organizationID, insightKey, ok := h.parseActionPath(w, r)
	if !ok {
		return
	}

	item, err := h.service.Reopen(r.Context(), organizationID, insightKey)
	if err != nil {
		writeInsightError(w, r, err)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func (h *InsightHandler) parseActionPath(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	organizationID, err := h.resolveOrganizationID(r)
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return "", "", false
	}
	insightKey := strings.TrimSpace(chi.URLParam(r, "insightKey"))
	if insightKey == "" {
		WriteError(w, r, http.StatusBadRequest, NewValidationError("request validation failed", ValidationIssue{
			Field:   "insightKey",
			Message: "is required",
		}))
		return "", "", false
	}
	return organizationID, insightKey, true
}

func (h *InsightHandler) resolveOrganizationID(r *http.Request) (string, *Error) {
	if pathValue := strings.TrimSpace(chi.URLParam(r, "organizationId")); pathValue != "" {
		return validateUUIDPathParam("organizationId", pathValue)
	}

	queryValue := strings.TrimSpace(r.URL.Query().Get("organizationId"))
	if queryValue == "" {
		return "", &Error{
			Code:    ErrorCodeValidation,
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "organizationId", Message: "is required"}},
		}
	}

	return validateUUIDPathParam("organizationId", queryValue)
}

func writeInsightError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, insights.ErrOrganizationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization not found"))
	case errors.Is(err, insights.ErrRepositoryNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Repository not found"))
	case errors.Is(err, insights.ErrInsightNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Insight not found"))
	default:
		var validationErr *insights.ValidationError
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
