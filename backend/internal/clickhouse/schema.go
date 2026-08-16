package clickhouse

import (
	"context"
	"fmt"
	"strings"

	"github.com/PangIkp/devlens/backend/internal/config"
)

type RetentionPolicy struct {
	AggregateDays int
}

func EnsureSchema(ctx context.Context, db *DB, cfg ...config.DataLifecycleConfig) error {
	databaseName := db.database
	if databaseName == "" {
		databaseName = "default"
	}

	if err := db.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", databaseName)); err != nil {
		return fmt.Errorf("ensure clickhouse database: %w", err)
	}

	policy := RetentionPolicy{AggregateDays: 365}
	if len(cfg) > 0 && cfg[0].AnalyticsAggregateRetentionDays > 0 {
		policy.AggregateDays = cfg[0].AnalyticsAggregateRetentionDays
	}

	for _, statement := range schemaStatements(policy) {
		if err := db.Exec(ctx, statement); err != nil {
			// "ALTER TABLE ... REMOVE TTL" is a one-time migration for
			// deployments that had the old global raw-data TTL — it's not
			// idempotent in ClickHouse (there's no REMOVE TTL IF EXISTS), so
			// once it has already run successfully once, every later
			// EnsureSchema call (e.g. on each app restart) would otherwise
			// fail here forever.
			if strings.HasPrefix(statement, "ALTER TABLE ") && strings.Contains(statement, "REMOVE TTL") &&
				strings.Contains(err.Error(), "doesn't have any table TTL expression") {
				continue
			}
			return fmt.Errorf("ensure clickhouse schema: %w", err)
		}
	}
	return nil
}

func schemaStatements(policy RetentionPolicy) []string {
	// Raw tables intentionally carry no table-level TTL: retention for them
	// is per-organization (organization_retention_settings.analytics_raw_retention_days),
	// which a single ClickHouse TTL expression can't express since it applies
	// to every row in the table regardless of which org it belongs to.
	// datalifecycle.Service.PurgeExpiredAnalyticsRawData enforces it instead,
	// deleting per organization on its own configured (or default) window.
	aggregateTTL := fmt.Sprintf("TTL toDateTime(metric_date) + INTERVAL %d DAY DELETE", policy.AggregateDays)

	return []string{
		`CREATE TABLE IF NOT EXISTS pull_requests (
		id String,
		repository_id String,
		github_pr_id Int64,
		number Int32,
		title String,
		author String,
		state String,
		created_at DateTime64(3, 'UTC'),
		merged_at Nullable(DateTime64(3, 'UTC')),
		closed_at Nullable(DateTime64(3, 'UTC')),
		additions Int32,
		deletions Int32,
		files_changed Int32,
		is_draft Bool,
		synced_at DateTime64(3, 'UTC')
	) ENGINE = ReplacingMergeTree(synced_at)
	PARTITION BY toYYYYMM(created_at)
	ORDER BY (repository_id, github_pr_id)`,
		`CREATE TABLE IF NOT EXISTS pull_request_reviews (
		id String,
		pull_request_id String,
		github_review_id Int64,
		reviewer String,
		review_requested_at Nullable(DateTime64(3, 'UTC')),
		first_review_at Nullable(DateTime64(3, 'UTC')),
		review_submitted_at Nullable(DateTime64(3, 'UTC')),
		state String,
		synced_at DateTime64(3, 'UTC')
	) ENGINE = ReplacingMergeTree(synced_at)
	PARTITION BY toYYYYMM(coalesce(review_submitted_at, first_review_at, review_requested_at, synced_at))
	ORDER BY (pull_request_id, github_review_id)`,
		`CREATE TABLE IF NOT EXISTS deployments (
		id String,
		repository_id String,
		environment String,
		status String,
		deployed_at DateTime64(3, 'UTC'),
		synced_at DateTime64(3, 'UTC')
	) ENGINE = ReplacingMergeTree(synced_at)
	PARTITION BY toYYYYMM(deployed_at)
	ORDER BY (repository_id, environment, deployed_at, id)`,
		`CREATE TABLE IF NOT EXISTS commit_events (
		id String,
		repository_id String,
		github_commit_sha String,
		author String,
		author_email Nullable(String),
		message String,
		authored_at DateTime64(3, 'UTC'),
		synced_at DateTime64(3, 'UTC')
	) ENGINE = ReplacingMergeTree(synced_at)
	PARTITION BY toYYYYMM(authored_at)
	ORDER BY (repository_id, github_commit_sha)`,
		`CREATE TABLE IF NOT EXISTS workflow_events (
		id String,
		repository_id String,
		github_workflow_run_id Int64,
		workflow_name String,
		status String,
		conclusion Nullable(String),
		started_at Nullable(DateTime64(3, 'UTC')),
		completed_at Nullable(DateTime64(3, 'UTC')),
		synced_at DateTime64(3, 'UTC')
	) ENGINE = ReplacingMergeTree(synced_at)
	PARTITION BY toYYYYMM(coalesce(started_at, completed_at, synced_at))
	ORDER BY (repository_id, github_workflow_run_id)`,
		`CREATE TABLE IF NOT EXISTS file_changes (
		id String,
		pull_request_id String,
		file_path String,
		additions Int32,
		deletions Int32,
		commit_count Int32,
		synced_at DateTime64(3, 'UTC')
	) ENGINE = ReplacingMergeTree(synced_at)
	ORDER BY (pull_request_id, file_path, id)`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS metrics_daily (
		metric_version UInt32,
		repository_id String,
		metric_date Date,
		pr_cycle_time_minutes Float64,
		review_wait_minutes Float64,
		average_review_minutes Float64,
		average_files_changed Float64,
		average_additions Float64,
		average_deletions Float64,
		deployment_frequency Float64,
		change_failure_rate Float64,
		review_coverage Float64,
		pr_count UInt64,
		merged_pr_count UInt64,
		reviewed_pr_count UInt64,
		review_wait_sample_count UInt64,
		review_time_sample_count UInt64,
		successful_deployment_count UInt64,
		failed_deployment_count UInt64,
		calculated_at DateTime64(3, 'UTC')
	) ENGINE = ReplacingMergeTree(calculated_at)
	PARTITION BY toYYYYMM(metric_date)
	ORDER BY (repository_id, metric_date)
	%s`, aggregateTTL),
		`ALTER TABLE metrics_daily ADD COLUMN IF NOT EXISTS metric_version UInt32 DEFAULT 1`,
		// Deployments that already ran EnsureSchema before per-organization
		// retention existed may have a table-level TTL from the old policy —
		// remove it so an org that configures a longer-than-default window
		// doesn't keep losing data to the old global TTL underneath it.
		`ALTER TABLE pull_requests REMOVE TTL`,
		`ALTER TABLE pull_request_reviews REMOVE TTL`,
		`ALTER TABLE deployments REMOVE TTL`,
		`ALTER TABLE commit_events REMOVE TTL`,
		`ALTER TABLE workflow_events REMOVE TTL`,
		`ALTER TABLE file_changes REMOVE TTL`,
		fmt.Sprintf(`ALTER TABLE metrics_daily MODIFY TTL toDateTime(metric_date) + INTERVAL %d DAY DELETE`, policy.AggregateDays),
	}
}
