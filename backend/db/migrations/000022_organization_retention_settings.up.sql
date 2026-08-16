CREATE TABLE organization_retention_settings (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id),
    analytics_raw_retention_days INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    updated_by UUID
);
