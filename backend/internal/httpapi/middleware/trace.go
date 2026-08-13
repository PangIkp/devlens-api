package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

const TraceIDHeader = "X-Trace-Id"

type traceIDContextKey struct{}

func TraceID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			tracer := otel.Tracer("devlens/http")
			spanName := strings.TrimSpace(r.Method) + " " + r.URL.Path
			ctx, span := tracer.Start(ctx, spanName)
			defer span.End()

			traceID := strings.TrimSpace(r.Header.Get(TraceIDHeader))
			if traceID == "" {
				if spanContext := span.SpanContext(); spanContext.IsValid() {
					traceID = spanContext.TraceID().String()
				}
			}
			if traceID == "" {
				traceID = newTraceID()
			}

			w.Header().Set(TraceIDHeader, traceID)
			ctx = context.WithValue(ctx, traceIDContextKey{}, traceID)
			span.SetAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.target", r.URL.Path),
				attribute.String("devlens.trace_id", traceID),
			)

			defer func() {
				if rec := recover(); rec != nil {
					span.SetStatus(codes.Error, "panic")
					panic(rec)
				}
			}()

			next.ServeHTTP(w, r.WithContext(ctx))
			span.SetStatus(codes.Ok, "")
		})
	}
}

func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(traceIDContextKey{}).(string)
	return value
}

func newTraceID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "trace-id-unavailable"
	}
	return hex.EncodeToString(buf[:])
}
