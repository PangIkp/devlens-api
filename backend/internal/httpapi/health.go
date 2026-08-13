package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"time"
)

type DependencyStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type HealthDependencies struct {
	Postgres   DependencyStatus `json:"postgres"`
	ClickHouse DependencyStatus `json:"clickhouse"`
	NATS       DependencyStatus `json:"nats"`
}

type HealthResponse struct {
	Status       string             `json:"status"`
	Timestamp    time.Time          `json:"timestamp"`
	Dependencies HealthDependencies `json:"dependencies"`
}

type healthMode string

const (
	healthModeLiveness  healthMode = "liveness"
	healthModeReadiness healthMode = "readiness"
)

type HealthHandler struct {
	mode       healthMode
	postgres   PostgresHealthChecker
	clickhouse ClickHouseHealthChecker
	nats       NATSHealthChecker
}

func NewHealthHandler(postgres PostgresHealthChecker, clickhouse ClickHouseHealthChecker, nats NATSHealthChecker) HealthHandler {
	return HealthHandler{
		mode:       healthModeLiveness,
		postgres:   postgres,
		clickhouse: clickhouse,
		nats:       nats,
	}
}

func NewReadinessHandler(postgres PostgresHealthChecker, clickhouse ClickHouseHealthChecker, nats NATSHealthChecker) HealthHandler {
	return HealthHandler{
		mode:       healthModeReadiness,
		postgres:   postgres,
		clickhouse: clickhouse,
		nats:       nats,
	}
}

func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
		Dependencies: HealthDependencies{
			Postgres:   DependencyStatus{Status: "ok"},
			ClickHouse: DependencyStatus{Status: "ok"},
			NATS:       DependencyStatus{Status: "ok"},
		},
	}

	statusCode := http.StatusOK

	if err := h.checkPostgres(r.Context()); err != nil {
		statusCode = http.StatusServiceUnavailable
		response.Status = "degraded"
		response.Dependencies.Postgres = DependencyStatus{
			Status:  "unavailable",
			Message: "PostgreSQL unavailable",
		}
	}

	if err := h.checkClickHouse(r.Context()); err != nil {
		statusCode = http.StatusServiceUnavailable
		response.Status = "degraded"
		response.Dependencies.ClickHouse = DependencyStatus{
			Status:  "unavailable",
			Message: "ClickHouse unavailable",
		}
	}

	if h.mode == healthModeReadiness {
		if err := h.checkNATS(r.Context()); err != nil {
			statusCode = http.StatusServiceUnavailable
			response.Status = "degraded"
			response.Dependencies.NATS = DependencyStatus{
				Status:  "unavailable",
				Message: "NATS unavailable",
			}
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

func (h HealthHandler) checkNATS(ctx context.Context) error {
	if isNilChecker(h.nats) {
		return nil
	}

	return h.nats.Check(ctx)
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
