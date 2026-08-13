package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/auth"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/go-chi/chi/v5"
)

type AuthService interface {
	Login(context.Context, auth.LoginRequest, auth.RequestMetadata) (auth.SessionResponse, error)
	Refresh(context.Context, auth.RefreshRequest) (auth.SessionResponse, error)
	Logout(context.Context, string) error
}

type AuthHandler struct {
	service AuthService
}

func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())
		r.Post("/auth/logout", h.logout)
	})
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	item, err := h.service.Login(r.Context(), req, auth.RequestMetadataFromHTTP(r))
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func (h *AuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var req auth.RefreshRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	item, err := h.service.Refresh(r.Context(), req)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized, NewUnauthorizedError("Authentication required"))
		return
	}
	if err := h.service.Logout(r.Context(), principal.SessionID); err != nil {
		writeAuthError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrSessionNotFound), errors.Is(err, auth.ErrSessionExpired):
		WriteError(w, r, http.StatusUnauthorized, NewUnauthorizedError("Authentication required"))
	default:
		var validationErr *auth.ValidationError
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
