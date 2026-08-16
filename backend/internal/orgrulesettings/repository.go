package orgrulesettings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository struct {
	db      *postgres.DB
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db, queries: db.Queries()}
}

func (r *Repository) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	var exists bool
	err := r.db.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE id = $1 AND deleted_at IS NULL)`, parseUUID(organizationID)).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check organization exists: %w", err)
	}
	if !exists {
		return ErrOrganizationNotFound
	}
	return nil
}

// Get returns the raw stored config JSON for an organization, or nil if the
// organization has never saved an override.
func (r *Repository) Get(ctx context.Context, organizationID string) ([]byte, *time.Time, error) {
	row, err := r.queries.GetOrganizationRuleSettings(ctx, parseUUID(organizationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("get organization rule settings: %w", err)
	}
	return row.ConfigJson, optionalTime(row.UpdatedAt), nil
}

func (r *Repository) Upsert(ctx context.Context, organizationID string, configJSON []byte, updatedBy *string) (*time.Time, error) {
	row, err := r.queries.UpsertOrganizationRuleSettings(ctx, sqlcgen.UpsertOrganizationRuleSettingsParams{
		OrganizationID: parseUUID(organizationID),
		ConfigJson:     configJSON,
		UpdatedBy:      nullableUUIDPtr(updatedBy),
	})
	if err != nil {
		return nil, fmt.Errorf("upsert organization rule settings: %w", err)
	}
	return optionalTime(row.UpdatedAt), nil
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func nullableUUIDPtr(value *string) pgtype.UUID {
	if value == nil || *value == "" {
		return pgtype.UUID{}
	}
	return parseUUID(*value)
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}
