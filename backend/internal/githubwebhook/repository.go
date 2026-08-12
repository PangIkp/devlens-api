package githubwebhook

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository struct {
	db      *postgres.DB
	queries *sqlcgen.Queries
}

type repositoryMatch struct {
	ID string
}

type enqueueResult struct {
	deliveryID string
	syncJobID  *string
	duplicate  bool
	receivedAt time.Time
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db, queries: db.Queries()}
}

func (r *Repository) FindRepositoryByGithubID(ctx context.Context, githubID int64) (*repositoryMatch, error) {
	if githubID < 1 {
		return nil, nil
	}

	row, err := r.queries.GetRepositoryByGithubID(ctx, githubID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find repository by github id: %w", err)
	}

	return &repositoryMatch{ID: row.ID.String()}, nil
}

func (r *Repository) EnqueueWebhookSync(ctx context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return enqueueResult{}, fmt.Errorf("begin webhook transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := r.queries.WithTx(tx)
	now := time.Now().UTC()
	payloadJSON, err := jsonValue(payload)
	if err != nil {
		return enqueueResult{}, err
	}
	installationUUID, err := r.findInstallationUUID(ctx, tx, installationID)
	if err != nil {
		return enqueueResult{}, err
	}

	var syncJobID *pgtype.UUID
	if enqueueJob && repositoryID != nil {
		row, err := queries.CreateSyncJob(ctx, sqlcgen.CreateSyncJobParams{
			ID:           newUUID(),
			RepositoryID: parseUUID(*repositoryID),
			Status:       "pending",
			Progress:     0,
			TriggeredBy:  pgtype.UUID{},
			ErrorMessage: pgtype.Text{},
			StartedAt:    pgtype.Timestamptz{},
			FinishedAt:   pgtype.Timestamptz{},
		})
		if err != nil {
			return enqueueResult{}, fmt.Errorf("create sync job from webhook: %w", err)
		}
		syncJobID = &row.ID
	}

	var createdDeliveryID pgtype.Text
	var createdSyncJobID pgtype.UUID
	var receivedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `
		INSERT INTO webhook_deliveries (
			id,
			repository_id,
			github_installation_id,
			github_delivery_id,
			event_type,
			processed,
			received_at,
			action,
			payload,
			sync_job_id,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9, $10
		)
		ON CONFLICT (github_delivery_id) DO NOTHING
		RETURNING github_delivery_id, sync_job_id, received_at`,
		newUUID(),
		nullableUUID(repositoryID),
		installationUUID,
		deliveryID,
		eventType,
		!enqueueJob,
		textPointerValue(action),
		payloadJSON,
		nullableUUIDFromValue(syncJobID),
		toNullableTimestamp(&now),
	).Scan(&createdDeliveryID, &createdSyncJobID, &receivedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := queries.GetWebhookDeliveryByGithubDeliveryID(ctx, textValue(deliveryID))
			if getErr != nil {
				return enqueueResult{}, fmt.Errorf("load duplicate webhook delivery: %w", getErr)
			}
			result := enqueueResult{
				deliveryID: deliveryID,
				duplicate:  true,
				receivedAt: existing.ReceivedAt.Time.UTC(),
			}
			if existing.SyncJobID.Valid {
				value := existing.SyncJobID.String()
				result.syncJobID = &value
			}
			return result, tx.Commit(ctx)
		}
		return enqueueResult{}, fmt.Errorf("create webhook delivery: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return enqueueResult{}, fmt.Errorf("commit webhook transaction: %w", err)
	}

	result := enqueueResult{
		deliveryID: deliveryID,
		duplicate:  false,
		receivedAt: receivedAt.Time.UTC(),
	}
	if createdSyncJobID.Valid {
		value := createdSyncJobID.String()
		result.syncJobID = &value
	}
	return result, nil
}

func newUUID() pgtype.UUID {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	return pgtype.UUID{Bytes: bytes, Valid: true}
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func nullableUUID(value *string) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return parseUUID(*value)
}

func nullableUUIDFromValue(value *pgtype.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return *value
}

func (r *Repository) findInstallationUUID(ctx context.Context, tx pgx.Tx, installationID *int64) (pgtype.UUID, error) {
	if installationID == nil || *installationID < 1 {
		return pgtype.UUID{}, nil
	}

	var id pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM github_installations WHERE installation_id = $1`, *installationID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, fmt.Errorf("find github installation uuid: %w", err)
	}
	return id, nil
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func textPointerValue(value *string) pgtype.Text {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Text{}
	}
	return textValue(strings.TrimSpace(*value))
}

func toNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func jsonValue(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return []byte("{}"), nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("decode webhook payload: %w", err)
	}
	return []byte(raw), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
