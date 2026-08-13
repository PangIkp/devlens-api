package middleware

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type HTTPMetrics struct {
	mu      sync.Mutex
	entries map[metricKey]*metricValue
}

type metricKey struct {
	Method string
	Path   string
	Status int
}

type metricValue struct {
	Count         uint64
	DurationSumMS int64
}

func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{
		entries: make(map[metricKey]*metricValue),
	}
}

func (m *HTTPMetrics) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m == nil {
				next.ServeHTTP(w, r)
				return
			}

			started := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			path := routePattern(r)
			m.record(metricKey{
				Method: r.Method,
				Path:   path,
				Status: status,
			}, time.Since(started))
		})
	}
}

func (m *HTTPMetrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(m.Render()))
	})
}

func (m *HTTPMetrics) Render() string {
	if m == nil {
		return ""
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	keys := make([]metricKey, 0, len(m.entries))
	for key := range m.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Path != keys[j].Path {
			return keys[i].Path < keys[j].Path
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Status < keys[j].Status
	})

	var builder strings.Builder
	builder.WriteString("# HELP devlens_http_requests_total Total HTTP requests.\n")
	builder.WriteString("# TYPE devlens_http_requests_total counter\n")
	for _, key := range keys {
		value := m.entries[key]
		builder.WriteString(fmt.Sprintf(
			"devlens_http_requests_total{method=%q,path=%q,status=%q} %d\n",
			key.Method,
			key.Path,
			strconv.Itoa(key.Status),
			value.Count,
		))
	}

	builder.WriteString("# HELP devlens_http_request_duration_ms_sum Total HTTP request duration in milliseconds.\n")
	builder.WriteString("# TYPE devlens_http_request_duration_ms_sum counter\n")
	for _, key := range keys {
		value := m.entries[key]
		builder.WriteString(fmt.Sprintf(
			"devlens_http_request_duration_ms_sum{method=%q,path=%q,status=%q} %d\n",
			key.Method,
			key.Path,
			strconv.Itoa(key.Status),
			value.DurationSumMS,
		))
	}

	builder.WriteString("# HELP devlens_http_request_duration_ms_count Count of HTTP requests included in duration totals.\n")
	builder.WriteString("# TYPE devlens_http_request_duration_ms_count counter\n")
	for _, key := range keys {
		value := m.entries[key]
		builder.WriteString(fmt.Sprintf(
			"devlens_http_request_duration_ms_count{method=%q,path=%q,status=%q} %d\n",
			key.Method,
			key.Path,
			strconv.Itoa(key.Status),
			value.Count,
		))
	}

	return builder.String()
}

func (m *HTTPMetrics) record(key metricKey, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	value, ok := m.entries[key]
	if !ok {
		value = &metricValue{}
		m.entries[key] = value
	}
	value.Count++
	value.DurationSumMS += duration.Milliseconds()
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "/unknown"
	}
	if r.URL != nil && r.URL.Path == "/metrics" {
		return "/metrics"
	}
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := strings.TrimSpace(rctx.RoutePattern()); pattern != "" {
			return pattern
		}
	}
	if r.URL != nil && strings.TrimSpace(r.URL.Path) != "" {
		return r.URL.Path
	}
	return "/unknown"
}
