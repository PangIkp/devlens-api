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
	GithubID int64
	Slug     string
	Name     string
}

type ListParams struct {
	Page     int
	PageSize int
}

type ListResult struct {
	Items      []OrganizationResponse
	TotalItems int
}

// Repository owns PostgreSQL access for organization use cases.
type Repository struct {
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{queries: db.Queries()}
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (OrganizationResponse, error) {
	row, err := r.queries.CreateOrganization(ctx, sqlcgen.CreateOrganizationParams{
		ID:       newUUID(),
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
	items, err := r.queries.ListOrganizations(ctx, sqlcgen.ListOrganizationsParams{
		Limit:  int32(params.PageSize),
		Offset: int32((params.Page - 1) * params.PageSize),
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list organizations: %w", err)
	}

	totalItems, err := r.queries.CountOrganizations(ctx)
	if err != nil {
		return ListResult{}, fmt.Errorf("count organizations: %w", err)
	}

	result := ListResult{
		Items:      make([]OrganizationResponse, 0, len(items)),
		TotalItems: int(totalItems),
	}
	for _, item := range items {
		result.Items = append(result.Items, toListResponse(item))
	}

	return result, nil
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
