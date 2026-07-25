package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/httpapi/middleware"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type PostgresHealthChecker interface {
	Check(context.Context) error
}

type Dependencies struct {
	Postgres PostgresHealthChecker
}

func NewRouter(logger *slog.Logger, deps Dependencies) http.Handler {
	router := chi.NewRouter()

	router.Use(chimiddleware.RequestID)
	router.Use(middleware.Recoverer(logger))
	router.Use(middleware.RequestLogger(logger))

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, Error{
			Code:    "NOT_FOUND",
			Message: "Resource not found",
		})
	})

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusMethodNotAllowed, Error{
			Code:    "METHOD_NOT_ALLOWED",
			Message: "Method not allowed",
		})
	})

	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", NewHealthHandler(deps.Postgres).ServeHTTP)
	})

	return router
}
