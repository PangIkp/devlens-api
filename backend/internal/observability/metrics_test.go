package observability

import (
	"strings"
	"testing"
	"time"
)

func TestMetricsRenderIncludesOperationalSeries(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()
	metrics.RecordQueueLag("metricsbus", "repository.sync.completed", 7)
	metrics.RecordSyncDuration("incremental", "completed", 2*time.Second)
	metrics.RecordWebhookProcessingDelay("push", "enqueued", 500*time.Millisecond)
	metrics.RecordClickHouseQuery("query", "ok", 150*time.Millisecond)

	output := metrics.Render()

	cases := []string{
		`devlens_queue_lag{component="metricsbus",subject="repository.sync.completed"} 7`,
		`devlens_sync_duration_ms_sum{mode="incremental",result="completed"} 2000`,
		`devlens_sync_duration_ms_count{mode="incremental",result="completed"} 1`,
		`devlens_webhook_processing_delay_ms_sum{event_type="push",status="enqueued"} 500`,
		`devlens_clickhouse_queries_total{operation="query",result="ok"} 1`,
	}

	for _, expected := range cases {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected metrics output to contain %q, got %q", expected, output)
		}
	}
}
