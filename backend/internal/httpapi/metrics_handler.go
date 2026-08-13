package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/go-chi/chi/v5"
)

type MetricsService interface {
	GetDashboardSummary(context.Context, string, metrics.QueryParams) (metrics.DashboardSummary, error)
	GetPullRequestMetrics(context.Context, string, metrics.QueryParams) (metrics.PullRequestMetrics, error)
	GetReviewMetrics(context.Context, string, metrics.QueryParams) (metrics.ReviewMetrics, error)
	GetDeploymentMetrics(context.Context, string, metrics.DeploymentQueryParams) (metrics.DeploymentMetrics, error)
	GetHotspots(context.Context, string, metrics.HotspotQueryParams) (metrics.HotspotResult, error)
	GetRepositoryMetrics(context.Context, string, metrics.DeploymentQueryParams) (metrics.RepositoryMetrics, error)
	GetReviewQueue(context.Context, string, metrics.HotspotQueryParams) (metrics.ReviewQueueResult, error)
}

type MetricsHandler struct {
	service    MetricsService
	authorizer AuthorizationService
}

func NewMetricsHandler(service MetricsService, authorizer ...AuthorizationService) *MetricsHandler {
	var authz AuthorizationService
	if len(authorizer) > 0 {
		authz = authorizer[0]
	}
	return &MetricsHandler{service: service, authorizer: authz}
}

func (h *MetricsHandler) RegisterRoutes(r chi.Router) {
	if h.authorizer == nil {
		r.Get("/repositories/{repositoryId}/dashboard/summary", h.getDashboardSummary)
		r.Get("/repositories/{repositoryId}/dashboard/pr-cycle-time", h.getPullRequestMetrics)
		r.Get("/repositories/{repositoryId}/dashboard/review-wait-time", h.getReviewMetrics)
		r.Get("/repositories/{repositoryId}/dashboard/review-queue", h.getReviewQueue)
		r.Get("/repositories/{repositoryId}/metrics", h.getRepositoryMetrics)
		r.Get("/repositories/{repositoryId}/metrics/pull-requests", h.getPullRequestMetrics)
		r.Get("/repositories/{repositoryId}/metrics/reviews", h.getReviewMetrics)
		r.Get("/repositories/{repositoryId}/metrics/deployments", h.getDeploymentMetrics)
		r.Get("/repositories/{repositoryId}/metrics/hotspots", h.getHotspots)
		return
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())
		withAccess := requireRepositoryPathRoles(h.authorizer, authorization.RoleMember, authorization.RoleAdmin, authorization.RoleOwner)
		r.With(withAccess).Get("/repositories/{repositoryId}/dashboard/summary", h.getDashboardSummary)
		r.With(withAccess).Get("/repositories/{repositoryId}/dashboard/pr-cycle-time", h.getPullRequestMetrics)
		r.With(withAccess).Get("/repositories/{repositoryId}/dashboard/review-wait-time", h.getReviewMetrics)
		r.With(withAccess).Get("/repositories/{repositoryId}/dashboard/review-queue", h.getReviewQueue)
		r.With(withAccess).Get("/repositories/{repositoryId}/metrics", h.getRepositoryMetrics)
		r.With(withAccess).Get("/repositories/{repositoryId}/metrics/pull-requests", h.getPullRequestMetrics)
		r.With(withAccess).Get("/repositories/{repositoryId}/metrics/reviews", h.getReviewMetrics)
		r.With(withAccess).Get("/repositories/{repositoryId}/metrics/deployments", h.getDeploymentMetrics)
		r.With(withAccess).Get("/repositories/{repositoryId}/metrics/hotspots", h.getHotspots)
	})
}

func (h *MetricsHandler) getReviewQueue(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	bounds, queryErr := parseDateRangeQuery(r)
	if queryErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(queryErr))
		return
	}

	page, pageErr := ParsePagination(r)
	if pageErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(pageErr))
		return
	}

	result, svcErr := h.service.GetReviewQueue(r.Context(), repositoryID, metrics.HotspotQueryParams{
		From:      bounds.From,
		To:        bounds.To,
		Page:      page.Page,
		PageSize:  page.PageSize,
		SortOrder: "asc",
	})
	if svcErr != nil {
		writeMetricsError(w, r, svcErr)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Data []metrics.ReviewQueueItem `json:"data"`
		Meta PaginationMeta            `json:"meta"`
	}{
		Data: result.Items,
		Meta: NewPaginationMeta(page.Page, page.PageSize, result.TotalItems),
	})
}

func (h *MetricsHandler) getDashboardSummary(w http.ResponseWriter, r *http.Request) {
	repositoryID, params, ok := h.parseRepositoryMetricsQuery(w, r, false)
	if !ok {
		return
	}

	response, err := h.service.GetDashboardSummary(r.Context(), repositoryID, params)
	if err != nil {
		writeMetricsError(w, r, err)
		return
	}

	WriteData(w, http.StatusOK, response)
}

func (h *MetricsHandler) getPullRequestMetrics(w http.ResponseWriter, r *http.Request) {
	repositoryID, params, ok := h.parseRepositoryMetricsQuery(w, r, true)
	if !ok {
		return
	}

	response, err := h.service.GetPullRequestMetrics(r.Context(), repositoryID, params)
	if err != nil {
		writeMetricsError(w, r, err)
		return
	}

	WriteData(w, http.StatusOK, response)
}

func (h *MetricsHandler) getRepositoryMetrics(w http.ResponseWriter, r *http.Request) {
	repositoryID, params, ok := h.parseRepositoryMetricsQuery(w, r, true)
	if !ok {
		return
	}

	var environment *string
	if value := strings.TrimSpace(r.URL.Query().Get("environment")); value != "" {
		environment = &value
	}

	response, err := h.service.GetRepositoryMetrics(r.Context(), repositoryID, metrics.DeploymentQueryParams{
		QueryParams: params,
		Environment: environment,
	})
	if err != nil {
		writeMetricsError(w, r, err)
		return
	}

	WriteData(w, http.StatusOK, response)
}

func (h *MetricsHandler) getReviewMetrics(w http.ResponseWriter, r *http.Request) {
	repositoryID, params, ok := h.parseRepositoryMetricsQuery(w, r, true)
	if !ok {
		return
	}

	response, err := h.service.GetReviewMetrics(r.Context(), repositoryID, params)
	if err != nil {
		writeMetricsError(w, r, err)
		return
	}

	WriteData(w, http.StatusOK, response)
}

func (h *MetricsHandler) getDeploymentMetrics(w http.ResponseWriter, r *http.Request) {
	repositoryID, params, ok := h.parseRepositoryMetricsQuery(w, r, true)
	if !ok {
		return
	}

	var environment *string
	if value := strings.TrimSpace(r.URL.Query().Get("environment")); value != "" {
		environment = &value
	}

	response, err := h.service.GetDeploymentMetrics(r.Context(), repositoryID, metrics.DeploymentQueryParams{
		QueryParams: params,
		Environment: environment,
	})
	if err != nil {
		writeMetricsError(w, r, err)
		return
	}

	WriteData(w, http.StatusOK, response)
}

func (h *MetricsHandler) getHotspots(w http.ResponseWriter, r *http.Request) {
	repositoryID, err := validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return
	}

	bounds, queryErr := parseDateRangeQuery(r)
	if queryErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(queryErr))
		return
	}

	page, pageErr := ParsePagination(r)
	if pageErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(pageErr))
		return
	}

	sortOrder := strings.TrimSpace(r.URL.Query().Get("sortOrder"))
	if sortOrder == "" {
		sortOrder = "desc"
	}

	result, svcErr := h.service.GetHotspots(r.Context(), repositoryID, metrics.HotspotQueryParams{
		From:      bounds.From,
		To:        bounds.To,
		Page:      page.Page,
		PageSize:  page.PageSize,
		SortOrder: sortOrder,
	})
	if svcErr != nil {
		writeMetricsError(w, r, svcErr)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Data []metrics.HotspotFile `json:"data"`
		Meta PaginationMeta        `json:"meta"`
	}{
		Data: result.Items,
		Meta: NewPaginationMeta(page.Page, page.PageSize, result.TotalItems),
	})
}

func (h *MetricsHandler) parseRepositoryMetricsQuery(w http.ResponseWriter, r *http.Request, withInterval bool) (string, metrics.QueryParams, bool) {
	repositoryID, err := validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, *err)
		return "", metrics.QueryParams{}, false
	}

	bounds, queryErr := parseDateRangeQuery(r)
	if queryErr != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(queryErr))
		return "", metrics.QueryParams{}, false
	}

	params := metrics.QueryParams{
		From: bounds.From,
		To:   bounds.To,
	}
	if withInterval {
		params.Interval = strings.TrimSpace(r.URL.Query().Get("interval"))
	}

	return repositoryID, params, true
}

func parseDateRangeQuery(r *http.Request) (metrics.QueryParams, error) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	if from == "" {
		return metrics.QueryParams{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{FieldRequired("from")},
		}
	}

	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if to == "" {
		return metrics.QueryParams{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{FieldRequired("to")},
		}
	}

	fromDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		return metrics.QueryParams{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{FieldInvalid("from", "must be a valid date in YYYY-MM-DD format")},
		}
	}

	toDate, err := time.Parse("2006-01-02", to)
	if err != nil {
		return metrics.QueryParams{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{FieldInvalid("to", "must be a valid date in YYYY-MM-DD format")},
		}
	}

	return metrics.QueryParams{From: fromDate.UTC(), To: toDate.UTC()}, nil
}

func writeMetricsError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, metrics.ErrRepositoryNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Repository not found"))
	default:
		var validationErr *metrics.ValidationError
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
