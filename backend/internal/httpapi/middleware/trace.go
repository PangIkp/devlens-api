package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

const TraceIDHeader = "X-Trace-Id"

type traceIDContextKey struct{}

func TraceID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := strings.TrimSpace(r.Header.Get(TraceIDHeader))
			if traceID == "" {
				traceID = newTraceID()
			}

			w.Header().Set(TraceIDHeader, traceID)
			ctx := context.WithValue(r.Context(), traceIDContextKey{}, traceID)
			next.ServeHTTP(w, r.WithContext(ctx))
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
