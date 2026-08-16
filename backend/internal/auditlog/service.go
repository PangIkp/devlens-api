package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

type Entry struct {
	OrganizationID *string
	ActorUserID    *string
	Action         string
	ResourceType   string
	ResourceID     *string
	Metadata       map[string]any
}

type Service struct {
	db  *postgres.DB
	now func() time.Time
}

func NewService(db *postgres.DB) *Service {
	return &Service{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Record(ctx context.Context, entry Entry) error {
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}

	_, err = s.db.Pool().Exec(ctx, `
INSERT INTO audit_logs (
    id,
    organization_id,
    actor_user_id,
    action,
    resource_type,
    resource_id,
    metadata_json,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		newUUID(),
		nullableUUID(entry.OrganizationID),
		nullableUUID(entry.ActorUserID),
		entry.Action,
		entry.ResourceType,
		nullableUUID(entry.ResourceID),
		metadata,
		s.now(),
	)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func nullableUUID(value *string) pgtype.UUID {
	if value == nil || *value == "" {
		return pgtype.UUID{}
	}
	var id pgtype.UUID
	_ = id.Scan(*value)
	return id
}

func newUUID() pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(genUUID())
	return id
}
