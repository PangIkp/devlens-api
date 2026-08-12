CREATE TABLE insight_statuses (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    repository_id UUID REFERENCES repositories(id) ON DELETE CASCADE,
    insight_key VARCHAR(255) NOT NULL,
    insight_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    evidence_json JSONB,
    reviewed_by UUID,
    reviewed_at TIMESTAMPTZ,
    dismissed_at TIMESTAMPTZ,
    reopened_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_insight_statuses_org_key
    ON insight_statuses (organization_id, insight_key);

CREATE INDEX idx_insight_statuses_repository_status
    ON insight_statuses (repository_id, status);
