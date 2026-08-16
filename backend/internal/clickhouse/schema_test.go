package clickhouse

import (
	"strings"
	"testing"
)

func TestSchemaStatementsUseDateTimeTTLExpressions(t *testing.T) {
	t.Parallel()

	statements := schemaStatements(RetentionPolicy{AggregateDays: 90})
	joined := strings.Join(statements, "\n")

	if !strings.Contains(joined, "TTL toDateTime(metric_date) + INTERVAL 90 DAY DELETE") {
		t.Fatal("expected aggregate TTL expressions to use toDateTime(metric_date)")
	}
}

func TestSchemaStatementsHaveNoTableLevelTTLOnRawTables(t *testing.T) {
	t.Parallel()

	statements := schemaStatements(RetentionPolicy{AggregateDays: 90})

	for _, table := range []string{"pull_requests", "pull_request_reviews", "deployments", "commit_events", "workflow_events", "file_changes"} {
		for _, statement := range statements {
			if strings.HasPrefix(statement, "CREATE TABLE IF NOT EXISTS "+table+" ") && strings.Contains(statement, "TTL") {
				t.Fatalf("expected %s CREATE TABLE statement to carry no TTL clause, retention is enforced per-organization instead: %s", table, statement)
			}
		}

		removeStatement := "ALTER TABLE " + table + " REMOVE TTL"
		found := false
		for _, statement := range statements {
			if statement == removeStatement {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected a %q migration statement to strip any TTL left by an older deployment", removeStatement)
		}
	}
}
