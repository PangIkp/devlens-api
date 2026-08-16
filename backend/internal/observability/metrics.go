package observability

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu                   sync.Mutex
	workerIterations     map[workerKey]uint64
	workerJobs           map[workerJobKey]uint64
	workerDuration       map[workerJobKey]int64
	queuePublishes       map[queueKey]uint64
	queueConsumes        map[queueKey]uint64
	queueProcessingCount map[queueKey]uint64
	queueProcessingSumMS map[queueKey]int64
	queueLag             map[queueLagKey]int64
	githubRequests       map[githubKey]uint64
	githubRequestSumMS   map[githubKey]int64
	githubRateLimit      map[string]int
	postgresQueries      map[postgresKey]uint64
	postgresQuerySumMS   map[postgresKey]int64
	clickhouseQueries    map[clickhouseKey]uint64
	clickhouseQuerySumMS map[clickhouseKey]int64
	syncDurations        map[syncKey]int64
	syncCounts           map[syncKey]uint64
	webhookDelayMS       map[webhookKey]int64
	webhookDelayCount    map[webhookKey]uint64
}

type workerKey struct {
	Worker string
	Result string
}

type workerJobKey struct {
	Worker string
	Status string
}

type queueKey struct {
	Component string
	Subject   string
	Result    string
}

type queueLagKey struct {
	Component string
	Subject   string
}

type githubKey struct {
	Method string
	Route  string
	Status string
	Result string
}

type postgresKey struct {
	Operation string
	Result    string
}

type clickhouseKey struct {
	Operation string
	Result    string
}

type syncKey struct {
	Mode   string
	Result string
}

type webhookKey struct {
	EventType string
	Status    string
}

func NewMetrics() *Metrics {
	return &Metrics{
		workerIterations:     make(map[workerKey]uint64),
		workerJobs:           make(map[workerJobKey]uint64),
		workerDuration:       make(map[workerJobKey]int64),
		queuePublishes:       make(map[queueKey]uint64),
		queueConsumes:        make(map[queueKey]uint64),
		queueProcessingCount: make(map[queueKey]uint64),
		queueProcessingSumMS: make(map[queueKey]int64),
		queueLag:             make(map[queueLagKey]int64),
		githubRequests:       make(map[githubKey]uint64),
		githubRequestSumMS:   make(map[githubKey]int64),
		githubRateLimit:      make(map[string]int),
		postgresQueries:      make(map[postgresKey]uint64),
		postgresQuerySumMS:   make(map[postgresKey]int64),
		clickhouseQueries:    make(map[clickhouseKey]uint64),
		clickhouseQuerySumMS: make(map[clickhouseKey]int64),
		syncDurations:        make(map[syncKey]int64),
		syncCounts:           make(map[syncKey]uint64),
		webhookDelayMS:       make(map[webhookKey]int64),
		webhookDelayCount:    make(map[webhookKey]uint64),
	}
}

func (m *Metrics) RecordWorkerIteration(worker, result string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workerIterations[workerKey{Worker: worker, Result: normalize(result)}]++
}

func (m *Metrics) RecordWorkerJob(worker, status string, duration time.Duration) {
	if m == nil {
		return
	}
	key := workerJobKey{Worker: worker, Status: normalize(status)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workerJobs[key]++
	m.workerDuration[key] += duration.Milliseconds()
}

func (m *Metrics) RecordQueuePublish(component, subject, result string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queuePublishes[queueKey{Component: component, Subject: subject, Result: normalize(result)}]++
}

func (m *Metrics) RecordQueueConsume(component, subject, result string, duration time.Duration) {
	if m == nil {
		return
	}
	key := queueKey{Component: component, Subject: subject, Result: normalize(result)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueConsumes[key]++
	m.queueProcessingCount[key]++
	m.queueProcessingSumMS[key] += duration.Milliseconds()
}

func (m *Metrics) RecordQueueLag(component, subject string, lag int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueLag[queueLagKey{Component: normalize(component), Subject: normalize(subject)}] = lag
}

func (m *Metrics) RecordGitHubRequest(method, route string, statusCode int, result string, duration time.Duration, remaining int) {
	if m == nil {
		return
	}
	key := githubKey{
		Method: strings.ToUpper(strings.TrimSpace(method)),
		Route:  normalizeRoute(route),
		Status: strconv.Itoa(statusCode),
		Result: normalize(result),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.githubRequests[key]++
	m.githubRequestSumMS[key] += duration.Milliseconds()
	if remaining >= 0 {
		m.githubRateLimit["core"] = remaining
	}
}

func (m *Metrics) RecordPostgresQuery(operation, result string, duration time.Duration) {
	if m == nil {
		return
	}
	key := postgresKey{Operation: normalize(operation), Result: normalize(result)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.postgresQueries[key]++
	m.postgresQuerySumMS[key] += duration.Milliseconds()
}

func (m *Metrics) RecordClickHouseQuery(operation, result string, duration time.Duration) {
	if m == nil {
		return
	}
	key := clickhouseKey{Operation: normalize(operation), Result: normalize(result)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clickhouseQueries[key]++
	m.clickhouseQuerySumMS[key] += duration.Milliseconds()
}

func (m *Metrics) RecordSyncDuration(mode, result string, duration time.Duration) {
	if m == nil {
		return
	}
	key := syncKey{Mode: normalize(mode), Result: normalize(result)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCounts[key]++
	m.syncDurations[key] += duration.Milliseconds()
}

func (m *Metrics) RecordWebhookProcessingDelay(eventType, status string, delay time.Duration) {
	if m == nil {
		return
	}
	key := webhookKey{EventType: normalize(eventType), Status: normalize(status)}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhookDelayCount[key]++
	m.webhookDelayMS[key] += delay.Milliseconds()
}

func (m *Metrics) Render() string {
	if m == nil {
		return ""
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var builder strings.Builder

	renderCounterMap(&builder,
		"devlens_worker_iterations_total",
		"Total worker iterations by worker and result.",
		[]string{"worker", "result"},
		mapToOrderedSeries(m.workerIterations, func(key workerKey) []string { return []string{key.Worker, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_worker_jobs_total",
		"Total worker jobs handled by worker and final status.",
		[]string{"worker", "status"},
		mapToOrderedSeries(m.workerJobs, func(key workerJobKey) []string { return []string{key.Worker, key.Status} }),
	)

	renderCounterMap(&builder,
		"devlens_worker_job_duration_ms_sum",
		"Total worker job duration in milliseconds.",
		[]string{"worker", "status"},
		mapToOrderedSeries64(m.workerDuration, func(key workerJobKey) []string { return []string{key.Worker, key.Status} }),
	)

	renderCounterMap(&builder,
		"devlens_queue_publish_total",
		"Total queue publish attempts.",
		[]string{"component", "subject", "result"},
		mapToOrderedSeries(m.queuePublishes, func(key queueKey) []string { return []string{key.Component, key.Subject, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_queue_consume_total",
		"Total queue messages consumed.",
		[]string{"component", "subject", "result"},
		mapToOrderedSeries(m.queueConsumes, func(key queueKey) []string { return []string{key.Component, key.Subject, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_queue_processing_duration_ms_sum",
		"Total queue message processing duration in milliseconds.",
		[]string{"component", "subject", "result"},
		mapToOrderedSeries64(m.queueProcessingSumMS, func(key queueKey) []string { return []string{key.Component, key.Subject, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_queue_processing_duration_ms_count",
		"Count of queue messages included in processing duration totals.",
		[]string{"component", "subject", "result"},
		mapToOrderedSeries(m.queueProcessingCount, func(key queueKey) []string { return []string{key.Component, key.Subject, key.Result} }),
	)

	renderGaugeMap(&builder,
		"devlens_queue_lag",
		"Latest observed queue lag by component and subject.",
		[]string{"component", "subject"},
		mapToOrderedSeries64(m.queueLag, func(key queueLagKey) []string { return []string{key.Component, key.Subject} }),
	)

	renderCounterMap(&builder,
		"devlens_github_requests_total",
		"Total GitHub API requests.",
		[]string{"method", "route", "status", "result"},
		mapToOrderedSeries(m.githubRequests, func(key githubKey) []string { return []string{key.Method, key.Route, key.Status, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_github_request_duration_ms_sum",
		"Total GitHub API request duration in milliseconds.",
		[]string{"method", "route", "status", "result"},
		mapToOrderedSeries64(m.githubRequestSumMS, func(key githubKey) []string { return []string{key.Method, key.Route, key.Status, key.Result} }),
	)

	renderGaugeMap(&builder,
		"devlens_github_rate_limit_remaining",
		"Latest observed GitHub API remaining rate limit budget.",
		[]string{"resource"},
		mapToOrderedSeriesInt(m.githubRateLimit, func(key string) []string { return []string{key} }),
	)

	renderCounterMap(&builder,
		"devlens_postgres_queries_total",
		"Total PostgreSQL queries executed.",
		[]string{"operation", "result"},
		mapToOrderedSeries(m.postgresQueries, func(key postgresKey) []string { return []string{key.Operation, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_postgres_query_duration_ms_sum",
		"Total PostgreSQL query duration in milliseconds.",
		[]string{"operation", "result"},
		mapToOrderedSeries64(m.postgresQuerySumMS, func(key postgresKey) []string { return []string{key.Operation, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_clickhouse_queries_total",
		"Total ClickHouse queries executed.",
		[]string{"operation", "result"},
		mapToOrderedSeries(m.clickhouseQueries, func(key clickhouseKey) []string { return []string{key.Operation, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_clickhouse_query_duration_ms_sum",
		"Total ClickHouse query duration in milliseconds.",
		[]string{"operation", "result"},
		mapToOrderedSeries64(m.clickhouseQuerySumMS, func(key clickhouseKey) []string { return []string{key.Operation, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_sync_duration_ms_sum",
		"Total sync duration in milliseconds by mode and result.",
		[]string{"mode", "result"},
		mapToOrderedSeries64(m.syncDurations, func(key syncKey) []string { return []string{key.Mode, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_sync_duration_ms_count",
		"Count of sync durations recorded by mode and result.",
		[]string{"mode", "result"},
		mapToOrderedSeries(m.syncCounts, func(key syncKey) []string { return []string{key.Mode, key.Result} }),
	)

	renderCounterMap(&builder,
		"devlens_webhook_processing_delay_ms_sum",
		"Total webhook processing delay in milliseconds by event type and resulting status.",
		[]string{"event_type", "status"},
		mapToOrderedSeries64(m.webhookDelayMS, func(key webhookKey) []string { return []string{key.EventType, key.Status} }),
	)

	renderCounterMap(&builder,
		"devlens_webhook_processing_delay_ms_count",
		"Count of webhook processing delay samples by event type and resulting status.",
		[]string{"event_type", "status"},
		mapToOrderedSeries(m.webhookDelayCount, func(key webhookKey) []string { return []string{key.EventType, key.Status} }),
	)

	return builder.String()
}

type series struct {
	labels []string
	value  int64
}

func mapToOrderedSeries[K comparable](items map[K]uint64, labels func(K) []string) []series {
	result := make([]series, 0, len(items))
	for key, value := range items {
		result = append(result, series{labels: labels(key), value: int64(value)})
	}
	sortSeries(result)
	return result
}

func mapToOrderedSeriesInt[K comparable](items map[K]int, labels func(K) []string) []series {
	result := make([]series, 0, len(items))
	for key, value := range items {
		result = append(result, series{labels: labels(key), value: int64(value)})
	}
	sortSeries(result)
	return result
}

func mapToOrderedSeries64[K comparable](items map[K]int64, labels func(K) []string) []series {
	result := make([]series, 0, len(items))
	for key, value := range items {
		result = append(result, series{labels: labels(key), value: value})
	}
	sortSeries(result)
	return result
}

func renderCounterMap(builder *strings.Builder, name, help string, labelNames []string, entries []series) {
	builder.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
	builder.WriteString(fmt.Sprintf("# TYPE %s counter\n", name))
	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf("%s%s %d\n", name, formatLabels(labelNames, entry.labels), entry.value))
	}
}

func renderGaugeMap(builder *strings.Builder, name, help string, labelNames []string, entries []series) {
	builder.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
	builder.WriteString(fmt.Sprintf("# TYPE %s gauge\n", name))
	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf("%s%s %d\n", name, formatLabels(labelNames, entry.labels), entry.value))
	}
}

func formatLabels(names, values []string) string {
	if len(names) == 0 || len(names) != len(values) {
		return ""
	}
	parts := make([]string, 0, len(names))
	for idx := range names {
		parts = append(parts, fmt.Sprintf(`%s=%q`, names[idx], values[idx]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func sortSeries(entries []series) {
	sort.Slice(entries, func(i, j int) bool {
		return strings.Join(entries[i].labels, "|") < strings.Join(entries[j].labels, "|")
	})
}

func normalize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func normalizeRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return "/unknown"
	}
	return route
}
