package organization

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrOrganizationConflict = errors.New("organization conflict")
)

type CreateParams struct {
	GithubID      int64
	Slug          string
	Name          string
	CreatorUserID string
}

type UpdateParams struct {
	ID   string
	Slug string
	Name string
}

type ListParams struct {
	UserID   string
	Page     int
	PageSize int
}

type ListResult struct {
	Items      []OrganizationResponse
	TotalItems int
}

// Repository owns PostgreSQL access for organization use cases.
type Repository struct {
	db      *postgres.DB
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db, queries: db.Queries()}
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (OrganizationResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return OrganizationResponse{}, fmt.Errorf("begin organization create transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := sqlcgen.New(tx)
	orgID := newUUID()
	row, err := queries.CreateOrganization(ctx, sqlcgen.CreateOrganizationParams{
		ID:       orgID,
		GithubID: params.GithubID,
		Slug:     params.Slug,
		Name:     textValue(params.Name),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return OrganizationResponse{}, ErrOrganizationConflict
		}
		return OrganizationResponse{}, fmt.Errorf("create organization: %w", err)
	}

	if _, err := queries.CreateOrganizationMember(ctx, sqlcgen.CreateOrganizationMemberParams{
		ID:             newUUID(),
		OrganizationID: orgID,
		UserID:         parseUUID(params.CreatorUserID),
		Role:           "owner",
	}); err != nil {
		return OrganizationResponse{}, fmt.Errorf("create owner membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return OrganizationResponse{}, fmt.Errorf("commit organization create transaction: %w", err)
	}

	return toCreateResponse(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (OrganizationResponse, error) {
	row, err := r.queries.GetOrganizationByID(ctx, parseUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrganizationResponse{}, ErrOrganizationNotFound
		}

		return OrganizationResponse{}, fmt.Errorf("get organization: %w", err)
	}

	return toGetResponse(row), nil
}

func (r *Repository) List(ctx context.Context, params ListParams) (ListResult, error) {
	rows, err := r.db.Pool().Query(ctx, `
SELECT o.id, o.github_id, o.slug, o.name, o.created_at, o.updated_at
FROM organizations o
INNER JOIN organization_members om ON om.organization_id = o.id
WHERE om.user_id = $1
  AND o.deleted_at IS NULL
ORDER BY o.created_at DESC
LIMIT $2 OFFSET $3`,
		parseUUID(params.UserID),
		params.PageSize,
		(params.Page-1)*params.PageSize,
	)
	if err != nil {
		return ListResult{}, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	result := ListResult{Items: make([]OrganizationResponse, 0)}
	for rows.Next() {
		var (
			id        pgtype.UUID
			name      pgtype.Text
			createdAt pgtype.Timestamptz
			updatedAt pgtype.Timestamptz
			item      OrganizationResponse
		)
		if err := rows.Scan(&id, &item.GithubID, &item.Slug, &name, &createdAt, &updatedAt); err != nil {
			return ListResult{}, fmt.Errorf("scan organization row: %w", err)
		}
		item.ID = uuidString(id)
		item.Name = name.String
		item.CreatedAt = timeValue(createdAt)
		item.UpdatedAt = optionalTimeValue(updatedAt)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("iterate organizations: %w", err)
	}

	if err := r.db.Pool().QueryRow(ctx, `
SELECT COUNT(*)
FROM organizations o
INNER JOIN organization_members om ON om.organization_id = o.id
WHERE om.user_id = $1
  AND o.deleted_at IS NULL`,
		parseUUID(params.UserID),
	).Scan(&result.TotalItems); err != nil {
		return ListResult{}, fmt.Errorf("count organizations: %w", err)
	}

	return result, nil
}

func (r *Repository) Update(ctx context.Context, params UpdateParams) (OrganizationResponse, error) {
	row, err := r.queries.UpdateOrganization(ctx, sqlcgen.UpdateOrganizationParams{
		ID:   parseUUID(params.ID),
		Slug: params.Slug,
		Name: textValue(params.Name),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrganizationResponse{}, ErrOrganizationNotFound
		}
		if isUniqueViolation(err) {
			return OrganizationResponse{}, ErrOrganizationConflict
		}

		return OrganizationResponse{}, fmt.Errorf("update organization: %w", err)
	}

	return toUpdateResponse(row), nil
}

func (r *Repository) SoftDelete(ctx context.Context, id string) error {
	_, err := r.queries.SoftDeleteOrganization(ctx, parseUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrganizationNotFound
		}

		return fmt.Errorf("soft delete organization: %w", err)
	}

	return nil
}

func toCreateResponse(row sqlcgen.CreateOrganizationRow) OrganizationResponse {
	return OrganizationResponse{
		ID:        uuidString(row.ID),
		GithubID:  row.GithubID,
		Slug:      row.Slug,
		Name:      row.Name.String,
		CreatedAt: timeValue(row.CreatedAt),
		UpdatedAt: optionalTimeValue(row.UpdatedAt),
	}
}

func toGetResponse(row sqlcgen.GetOrganizationByIDRow) OrganizationResponse {
	return OrganizationResponse{
		ID:        uuidString(row.ID),
		GithubID:  row.GithubID,
		Slug:      row.Slug,
		Name:      row.Name.String,
		CreatedAt: timeValue(row.CreatedAt),
		UpdatedAt: optionalTimeValue(row.UpdatedAt),
	}
}

func toListResponse(row sqlcgen.ListOrganizationsRow) OrganizationResponse {
	return OrganizationResponse{
		ID:        uuidString(row.ID),
		GithubID:  row.GithubID,
		Slug:      row.Slug,
		Name:      row.Name.String,
		CreatedAt: timeValue(row.CreatedAt),
		UpdatedAt: optionalTimeValue(row.UpdatedAt),
	}
}

func toUpdateResponse(row sqlcgen.UpdateOrganizationRow) OrganizationResponse {
	return OrganizationResponse{
		ID:        uuidString(row.ID),
		GithubID:  row.GithubID,
		Slug:      row.Slug,
		Name:      row.Name.String,
		CreatedAt: timeValue(row.CreatedAt),
		UpdatedAt: optionalTimeValue(row.UpdatedAt),
	}
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

func uuidString(value pgtype.UUID) string {
	return value.String()
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func optionalTimeValue(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}

	utc := value.Time.UTC()
	return &utc
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
