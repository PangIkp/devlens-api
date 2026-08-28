CREATE TABLE IF NOT EXISTS metrics_daily (
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    metric_version INTEGER NOT NULL DEFAULT 1,
    metric_date DATE NOT NULL,
    pr_cycle_time_minutes DOUBLE PRECISION NOT NULL DEFAULT 0,
    review_wait_minutes DOUBLE PRECISION NOT NULL DEFAULT 0,
    average_review_minutes DOUBLE PRECISION NOT NULL DEFAULT 0,
    average_files_changed DOUBLE PRECISION NOT NULL DEFAULT 0,
    average_additions DOUBLE PRECISION NOT NULL DEFAULT 0,
    average_deletions DOUBLE PRECISION NOT NULL DEFAULT 0,
    deployment_frequency DOUBLE PRECISION NOT NULL DEFAULT 0,
    change_failure_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    review_coverage DOUBLE PRECISION NOT NULL DEFAULT 0,
    pr_count BIGINT NOT NULL DEFAULT 0,
    merged_pr_count BIGINT NOT NULL DEFAULT 0,
    reviewed_pr_count BIGINT NOT NULL DEFAULT 0,
    review_wait_sample_count BIGINT NOT NULL DEFAULT 0,
    review_time_sample_count BIGINT NOT NULL DEFAULT 0,
    successful_deployment_count BIGINT NOT NULL DEFAULT 0,
    failed_deployment_count BIGINT NOT NULL DEFAULT 0,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (repository_id, metric_version, metric_date)
);

CREATE INDEX IF NOT EXISTS idx_metrics_daily_repository_metric_date
    ON metrics_daily (repository_id, metric_date);
