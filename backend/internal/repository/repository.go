package repository

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
	ErrRepositoryNotFound   = errors.New("repository not found")
	ErrRepositoryConflict   = errors.New("repository conflict")
)

type CreateParams struct {
	OrganizationID string
	GithubID       int64
	Name           string
	FullName       string
	DefaultBranch  *string
}

type UpdateParams struct {
	ID            string
	Name          string
	FullName      string
	DefaultBranch *string
	IsActive      bool
	ArchivedAt    *time.Time
}

type Repository struct {
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{queries: db.Queries()}
}

func (r *Repository) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	exists, err := r.queries.RepositoryOrganizationExists(ctx, parseUUID(organizationID))
	if err != nil {
		return fmt.Errorf("check organization exists: %w", err)
	}
	if !exists {
		return ErrOrganizationNotFound
	}
	return nil
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (RepositoryResponse, error) {
	row, err := r.queries.CreateRepository(ctx, sqlcgen.CreateRepositoryParams{
		ID:             newUUID(),
		OrganizationID: parseUUID(params.OrganizationID),
		GithubID:       params.GithubID,
		Name:           params.Name,
		FullName:       params.FullName,
		DefaultBranch:  textValue(params.DefaultBranch),
		IsActive:       true,
		ArchivedAt:     toNullableTimestamp(nil),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return RepositoryResponse{}, ErrRepositoryConflict
		}
		return RepositoryResponse{}, fmt.Errorf("create repository: %w", err)
	}

	return toCreateResponse(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (RepositoryResponse, error) {
	row, err := r.queries.GetRepositoryByID(ctx, parseUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RepositoryResponse{}, ErrRepositoryNotFound
		}
		return RepositoryResponse{}, fmt.Errorf("get repository: %w", err)
	}

	return toGetResponse(row), nil
}

func (r *Repository) List(ctx context.Context, params ListParams) (ListResult, error) {
	rows, err := r.queries.ListRepositories(ctx, sqlcgen.ListRepositoriesParams{
		OrganizationID: parseUUID(params.OrganizationID),
		Column2:        params.Status,
		Column3:        params.Search,
		Column4:        params.SortBy,
		Column5:        params.SortOrder,
		Limit:          int32(params.PageSize),
		Offset:         int32((params.Page - 1) * params.PageSize),
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list repositories: %w", err)
	}

	totalItems, err := r.queries.CountRepositories(ctx, sqlcgen.CountRepositoriesParams{
		OrganizationID: parseUUID(params.OrganizationID),
		Column2:        params.Status,
		Column3:        params.Search,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("count repositories: %w", err)
	}

	result := ListResult{
		Items:      make([]RepositoryResponse, 0, len(rows)),
		TotalItems: int(totalItems),
	}
	for _, row := range rows {
		result.Items = append(result.Items, toListResponse(row))
	}

	return result, nil
}

func (r *Repository) Update(ctx context.Context, params UpdateParams) (RepositoryResponse, error) {
	row, err := r.queries.UpdateRepository(ctx, sqlcgen.UpdateRepositoryParams{
		ID:            parseUUID(params.ID),
		Name:          params.Name,
		FullName:      params.FullName,
		DefaultBranch: textValue(params.DefaultBranch),
		IsActive:      params.IsActive,
		ArchivedAt:    toNullableTimestamp(params.ArchivedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RepositoryResponse{}, ErrRepositoryNotFound
		}
		if isUniqueViolation(err) {
			return RepositoryResponse{}, ErrRepositoryConflict
		}
		return RepositoryResponse{}, fmt.Errorf("update repository: %w", err)
	}

	return toUpdateResponse(row), nil
}

func toCreateResponse(row sqlcgen.CreateRepositoryRow) RepositoryResponse {
	defaultBranch := optionalTextValue(row.DefaultBranch)
	return RepositoryResponse{
		ID:             row.ID.String(),
		OrganizationID: row.OrganizationID.String(),
		GithubID:       row.GithubID,
		Name:           row.Name,
		FullName:       row.FullName,
		DefaultBranch:  defaultBranch,
		IsActive:       row.IsActive,
		ArchivedAt:     optionalTimeValue(row.ArchivedAt),
		LastSyncedAt:   optionalTimeValue(row.LastSyncedAt),
		CreatedAt:      row.CreatedAt.Time.UTC(),
		UpdatedAt:      optionalTimeValue(row.UpdatedAt),
	}
}

func toGetResponse(row sqlcgen.GetRepositoryByIDRow) RepositoryResponse {
	defaultBranch := optionalTextValue(row.DefaultBranch)
	return RepositoryResponse{
		ID:             row.ID.String(),
		OrganizationID: row.OrganizationID.String(),
		GithubID:       row.GithubID,
		Name:           row.Name,
		FullName:       row.FullName,
		DefaultBranch:  defaultBranch,
		IsActive:       row.IsActive,
		ArchivedAt:     optionalTimeValue(row.ArchivedAt),
		LastSyncedAt:   optionalTimeValue(row.LastSyncedAt),
		CreatedAt:      row.CreatedAt.Time.UTC(),
		UpdatedAt:      optionalTimeValue(row.UpdatedAt),
	}
}

func toListResponse(row sqlcgen.ListRepositoriesRow) RepositoryResponse {
	defaultBranch := optionalTextValue(row.DefaultBranch)
	return RepositoryResponse{
		ID:             row.ID.String(),
		OrganizationID: row.OrganizationID.String(),
		GithubID:       row.GithubID,
		Name:           row.Name,
		FullName:       row.FullName,
		DefaultBranch:  defaultBranch,
		IsActive:       row.IsActive,
		ArchivedAt:     optionalTimeValue(row.ArchivedAt),
		LastSyncedAt:   optionalTimeValue(row.LastSyncedAt),
		CreatedAt:      row.CreatedAt.Time.UTC(),
		UpdatedAt:      optionalTimeValue(row.UpdatedAt),
	}
}

func toUpdateResponse(row sqlcgen.UpdateRepositoryRow) RepositoryResponse {
	defaultBranch := optionalTextValue(row.DefaultBranch)
	return RepositoryResponse{
		ID:             row.ID.String(),
		OrganizationID: row.OrganizationID.String(),
		GithubID:       row.GithubID,
		Name:           row.Name,
		FullName:       row.FullName,
		DefaultBranch:  defaultBranch,
		IsActive:       row.IsActive,
		ArchivedAt:     optionalTimeValue(row.ArchivedAt),
		LastSyncedAt:   optionalTimeValue(row.LastSyncedAt),
		CreatedAt:      row.CreatedAt.Time.UTC(),
		UpdatedAt:      optionalTimeValue(row.UpdatedAt),
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

func textValue(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalTextValue(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func toNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
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
