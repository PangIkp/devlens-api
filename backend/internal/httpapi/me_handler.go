package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/auth"
	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/PangIkp/devlens/backend/internal/userprofile"
	"github.com/go-chi/chi/v5"
)

type MeService interface {
	Get(context.Context, string) (userprofile.Response, error)
}

type MeHandler struct {
	service MeService
}

func NewMeHandler(service MeService) *MeHandler {
	return &MeHandler{service: service}
}

func (h *MeHandler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth())
		r.Get("/me", h.get)
	})
}

func (h *MeHandler) get(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized, NewUnauthorizedError("Authentication required"))
		return
	}

	userID := principal.User.ID
	item, err := h.service.Get(r.Context(), userID)
	if err != nil {
		writeMeError(w, r, err)
		return
	}
	WriteData(w, http.StatusOK, item)
}

func writeMeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, userprofile.ErrUserNotFound):
		WriteError(w, r, http.StatusNotFound, NewNotFoundError("User not found"))
	default:
		var validationErr *userprofile.ValidationError
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
