package clickhouse

import (
	"strings"
	"testing"
)

func TestSchemaStatementsUseDateTimeTTLExpressions(t *testing.T) {
	t.Parallel()

	statements := schemaStatements(RetentionPolicy{RawDays: 30, AggregateDays: 90})
	joined := strings.Join(statements, "\n")

	if strings.Contains(joined, "TTL synced_at + INTERVAL") {
		t.Fatal("expected raw TTL expressions to cast synced_at to DateTime")
	}
	if !strings.Contains(joined, "TTL toDateTime(synced_at) + INTERVAL 30 DAY DELETE") {
		t.Fatal("expected raw TTL expressions to use toDateTime(synced_at)")
	}
	if !strings.Contains(joined, "TTL toDateTime(metric_date) + INTERVAL 90 DAY DELETE") {
		t.Fatal("expected aggregate TTL expressions to use toDateTime(metric_date)")
	}
}
