package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/auth"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/go-chi/chi/v5"
)

type AuthorizationService interface {
	AuthorizeOrganization(context.Context, string, string, ...string) error
	AuthorizeRepository(context.Context, string, string, ...string) error
	AuthorizePullRequest(context.Context, string, string, ...string) error
	AuthorizeSyncJob(context.Context, string, string, ...string) error
}

type resourceExtractor func(*http.Request) (string, *Error)
type authorizeFunc func(context.Context, string, string, ...string) error

func requireAuthenticated(next http.Handler) http.Handler {
	return middleware.RequireAuth()(next)
}

func requireOrganizationRoles(authorizer AuthorizationService, roles ...string) func(http.Handler) http.Handler {
	return requireResourceRoles(
		authorizer,
		func(r *http.Request) (string, *Error) {
			return validateUUIDPathParam("organizationId", chi.URLParam(r, "organizationId"))
		},
		func(ctx context.Context, userID string, resourceID string, roles ...string) error {
			return authorizer.AuthorizeOrganization(ctx, userID, resourceID, roles...)
		},
		roles...,
	)
}

func requireOrganizationQueryRoles(authorizer AuthorizationService, roles ...string) func(http.Handler) http.Handler {
	return requireResourceRoles(
		authorizer,
		func(r *http.Request) (string, *Error) {
			value := strings.TrimSpace(r.URL.Query().Get("organizationId"))
			if value == "" {
				err := NewValidationError("request validation failed", ValidationIssue{Field: "organizationId", Message: "is required"})
				return "", &err
			}
			return validateUUIDPathParam("organizationId", value)
		},
		func(ctx context.Context, userID string, resourceID string, roles ...string) error {
			return authorizer.AuthorizeOrganization(ctx, userID, resourceID, roles...)
		},
		roles...,
	)
}

func requireRepositoryPathRoles(authorizer AuthorizationService, roles ...string) func(http.Handler) http.Handler {
	return requireResourceRoles(
		authorizer,
		func(r *http.Request) (string, *Error) {
			return validateUUIDPathParam("repositoryId", chi.URLParam(r, "repositoryId"))
		},
		func(ctx context.Context, userID string, resourceID string, roles ...string) error {
			return authorizer.AuthorizeRepository(ctx, userID, resourceID, roles...)
		},
		roles...,
	)
}

func requireRepositoryQueryRoles(authorizer AuthorizationService, roles ...string) func(http.Handler) http.Handler {
	return requireResourceRoles(
		authorizer,
		func(r *http.Request) (string, *Error) {
			value := strings.TrimSpace(r.URL.Query().Get("repositoryId"))
			if value == "" {
				err := NewValidationError("request validation failed", ValidationIssue{Field: "repositoryId", Message: "is required"})
				return "", &err
			}
			return validateUUIDPathParam("repositoryId", value)
		},
		func(ctx context.Context, userID string, resourceID string, roles ...string) error {
			return authorizer.AuthorizeRepository(ctx, userID, resourceID, roles...)
		},
		roles...,
	)
}

func requirePullRequestRoles(authorizer AuthorizationService, roles ...string) func(http.Handler) http.Handler {
	return requireResourceRoles(
		authorizer,
		func(r *http.Request) (string, *Error) {
			return validateUUIDPathParam("pullRequestId", chi.URLParam(r, "pullRequestId"))
		},
		func(ctx context.Context, userID string, resourceID string, roles ...string) error {
			return authorizer.AuthorizePullRequest(ctx, userID, resourceID, roles...)
		},
		roles...,
	)
}

func requireSyncJobRoles(authorizer AuthorizationService, roles ...string) func(http.Handler) http.Handler {
	return requireResourceRoles(
		authorizer,
		func(r *http.Request) (string, *Error) {
			return validateUUIDPathParam("syncJobId", chi.URLParam(r, "syncJobId"))
		},
		func(ctx context.Context, userID string, resourceID string, roles ...string) error {
			return authorizer.AuthorizeSyncJob(ctx, userID, resourceID, roles...)
		},
		roles...,
	)
}

func requireResourceRoles(authorizer AuthorizationService, extract resourceExtractor, authorize authorizeFunc, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		next = requireAuthenticated(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				WriteError(w, r, http.StatusUnauthorized, NewUnauthorizedError("Authentication required"))
				return
			}
			resourceID, err := extract(r)
			if err != nil {
				WriteError(w, r, http.StatusBadRequest, *err)
				return
			}
			if authorizer != nil {
				if authzErr := authorize(r.Context(), principal.User.ID, resourceID, roles...); authzErr != nil {
					writeAuthorizationError(w, r, authzErr)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func currentUserID(r *http.Request) (string, bool) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		return "", false
	}
	return principal.User.ID, true
}

func writeAuthorizationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, authorization.ErrForbidden):
		WriteError(w, r, http.StatusForbidden, NewForbiddenError("You do not have permission to access this resource"))
	case errors.Is(err, authorization.ErrOrganizationNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Organization not found"))
	case errors.Is(err, authorization.ErrRepositoryNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Repository not found"))
	case errors.Is(err, authorization.ErrPullRequestNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Pull request not found"))
	case errors.Is(err, authorization.ErrSyncJobNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("Sync job not found"))
	default:
		WriteError(w, r, http.StatusInternalServerError, NewInternalError())
	}
}
