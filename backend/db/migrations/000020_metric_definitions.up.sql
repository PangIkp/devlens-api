CREATE TABLE metric_definitions (
    id UUID PRIMARY KEY,
    metric_key VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    metric_version INTEGER NOT NULL,
    algorithm_version VARCHAR(255),
    unit VARCHAR(64),
    description TEXT,
    config_json JSONB,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,
    updated_at TIMESTAMPTZ,
    updated_by UUID,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID
);

CREATE UNIQUE INDEX metric_definitions_metric_key_metric_version_idx
    ON metric_definitions (metric_key, metric_version);

CREATE UNIQUE INDEX metric_definitions_active_metric_key_idx
    ON metric_definitions (metric_key)
    WHERE is_active = TRUE AND deleted_at IS NULL;
