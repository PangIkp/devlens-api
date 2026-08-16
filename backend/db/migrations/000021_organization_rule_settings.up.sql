CREATE TABLE organization_rule_settings (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id),
    config_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ,
    updated_by UUID
);
