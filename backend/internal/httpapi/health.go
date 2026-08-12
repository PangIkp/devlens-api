package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"time"
)

type PostgresDependencyStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type ClickHouseDependencyStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type HealthDependencies struct {
	Postgres   PostgresDependencyStatus   `json:"postgres"`
	ClickHouse ClickHouseDependencyStatus `json:"clickhouse"`
}

type HealthResponse struct {
	Status       string             `json:"status"`
	Timestamp    time.Time          `json:"timestamp"`
	Dependencies HealthDependencies `json:"dependencies"`
}

type HealthHandler struct {
	postgres   PostgresHealthChecker
	clickhouse ClickHouseHealthChecker
}

func NewHealthHandler(postgres PostgresHealthChecker, clickhouse ClickHouseHealthChecker) HealthHandler {
	return HealthHandler{postgres: postgres, clickhouse: clickhouse}
}

func NewReadinessHandler(postgres PostgresHealthChecker, clickhouse ClickHouseHealthChecker) HealthHandler {
	return HealthHandler{postgres: postgres, clickhouse: clickhouse}
}

func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
		Dependencies: HealthDependencies{
			Postgres:   PostgresDependencyStatus{Status: "ok"},
			ClickHouse: ClickHouseDependencyStatus{Status: "ok"},
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

	if err := h.checkClickHouse(r.Context()); err != nil {
		statusCode = http.StatusServiceUnavailable
		response.Status = "degraded"
		response.Dependencies.ClickHouse = ClickHouseDependencyStatus{
			Status:  "unavailable",
			Message: "ClickHouse unavailable",
		}
	}

	writeJSON(w, statusCode, response)
}

func (h HealthHandler) checkPostgres(ctx context.Context) error {
	if isNilChecker(h.postgres) {
		return nil
	}

	return h.postgres.Check(ctx)
}

func (h HealthHandler) checkClickHouse(ctx context.Context) error {
	if isNilChecker(h.clickhouse) {
		return nil
	}

	return h.clickhouse.Check(ctx)
}

func isNilChecker(checker any) bool {
	if checker == nil {
		return true
	}

	value := reflect.ValueOf(checker)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
