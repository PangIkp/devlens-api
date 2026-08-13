package clickhouse

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
)

type integrationRow struct {
	ID    string `json:"id"`
	Value int64  `json:"value"`
}

func TestEnsureSchemaAndQueryJSONEachRowIntegration(t *testing.T) {
	t.Parallel()

	db := openIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := EnsureSchema(ctx, db); err != nil {
		t.Skipf("skip clickhouse integration test: ensure schema failed: %v", err)
	}

	tableName := fmt.Sprintf("integration_test_%d", time.Now().UTC().UnixNano())
	t.Cleanup(func() {
		_ = db.Exec(context.Background(), "DROP TABLE IF EXISTS "+tableName)
	})

	if err := db.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id String,
	value Int64
) ENGINE = MergeTree
ORDER BY id
`, tableName)); err != nil {
		t.Fatalf("create test table: %v", err)
	}

	rows := []integrationRow{
		{ID: "row-1", Value: 10},
		{ID: "row-2", Value: 20},
	}
	if err := db.InsertJSONEachRow(ctx, "INSERT INTO "+tableName, rows); err != nil {
		t.Fatalf("insert test rows: %v", err)
	}

	result, err := QueryJSONEachRow[integrationRow](ctx, db, fmt.Sprintf(`
SELECT id, value
FROM %s
ORDER BY id ASC
`, tableName))
	if err != nil {
		t.Fatalf("query test rows: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	if result[0].ID != "row-1" || result[0].Value != 10 {
		t.Fatalf("unexpected first row %+v", result[0])
	}
	if result[1].ID != "row-2" || result[1].Value != 20 {
		t.Fatalf("unexpected second row %+v", result[1])
	}
}

func openIntegrationDB(t *testing.T) *DB {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	db, err := Open(cfg.ClickHouse, nil)
	if err != nil {
		t.Skipf("skip clickhouse integration test: clickhouse unavailable: %v", err)
	}

	return db
}
