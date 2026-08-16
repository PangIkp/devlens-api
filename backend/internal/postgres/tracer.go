package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/observability"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type queryStateContextKey struct{}
type querySpanContextKey struct{}

type queryTraceState struct {
	started   time.Time
	operation string
}

type queryTracer struct {
	metrics *observability.Metrics
}

func newQueryTracer(metrics *observability.Metrics) *queryTracer {
	return &queryTracer{metrics: metrics}
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	operation := sqlOperation(data.SQL)
	ctx, span := otel.Tracer("devlens/postgres").Start(ctx, "postgres."+operation)
	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
	)
	ctx = context.WithValue(ctx, queryStateContextKey{}, queryTraceState{
		started:   time.Now(),
		operation: operation,
	})
	return context.WithValue(ctx, querySpanContextKey{}, span)
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, _ := ctx.Value(querySpanContextKey{}).(trace.Span)
	state, _ := ctx.Value(queryStateContextKey{}).(queryTraceState)

	duration := time.Since(state.started)
	result := "ok"
	if data.Err != nil {
		result = "error"
	}
	if t.metrics != nil {
		t.metrics.RecordPostgresQuery(state.operation, result, duration)
	}

	if span != nil {
		span.SetAttributes(attribute.Int64("devlens.postgres.query_ms", duration.Milliseconds()))
		if data.Err != nil {
			span.SetStatus(codes.Error, data.Err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}

func sqlOperation(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "unknown"
	}
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return "unknown"
	}
	return strings.ToLower(fields[0])
}
