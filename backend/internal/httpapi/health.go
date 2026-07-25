package httpapi

import (
	"context"
	"net/http"
	"time"
)

type PostgresDependencyStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type HealthDependencies struct {
	Postgres PostgresDependencyStatus `json:"postgres"`
}

type HealthResponse struct {
	Status       string             `json:"status"`
	Timestamp    time.Time          `json:"timestamp"`
	Dependencies HealthDependencies `json:"dependencies"`
}

type HealthHandler struct {
	postgres PostgresHealthChecker
}

func NewHealthHandler(postgres PostgresHealthChecker) HealthHandler {
	return HealthHandler{postgres: postgres}
}

func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
		Dependencies: HealthDependencies{
			Postgres: PostgresDependencyStatus{Status: "ok"},
		},
	}

	statusCode := http.StatusOK

	if err := h.checkPostgres(r.Context()); err != nil {
		statusCode = http.StatusServiceUnavailable
		response.Status = "degraded"
		response.Dependencies.Postgres = PostgresDependencyStatus{
			Status:  "unavailable",
			Message: "PostgreSQL unavailable",
		}
	}

	writeJSON(w, statusCode, response)
}

func (h HealthHandler) checkPostgres(ctx context.Context) error {
	if h.postgres == nil {
		return nil
	}

	return h.postgres.Check(ctx)
}
