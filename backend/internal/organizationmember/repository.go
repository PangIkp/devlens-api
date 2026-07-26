package organizationmember

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrMemberNotFound       = errors.New("organization member not found")
	ErrMemberConflict       = errors.New("organization member conflict")
	ErrLastOwnerConflict    = errors.New("cannot remove last owner")
	ErrUserNotFound         = errors.New("user not found")
)

type CreateParams struct {
	OrganizationID string
	UserID         string
	Role           string
}

type UpdateParams struct {
	ID   string
	Role string
}

type Repository struct {
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{queries: db.Queries()}
}

func (r *Repository) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	exists, err := r.queries.OrganizationExists(ctx, parseUUID(organizationID))
	if err != nil {
		return fmt.Errorf("check organization exists: %w", err)
	}
	if !exists {
		return ErrOrganizationNotFound
	}
	return nil
}

func (r *Repository) EnsureUserExists(ctx context.Context, userID string) error {
	exists, err := r.queries.UserExists(ctx, parseUUID(userID))
	if err != nil {
		return fmt.Errorf("check user exists: %w", err)
	}
	if !exists {
		return ErrUserNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, organizationID string) ([]MemberResponse, error) {
	rows, err := r.queries.ListOrganizationMembers(ctx, parseUUID(organizationID))
	if err != nil {
		return nil, fmt.Errorf("list organization members: %w", err)
	}

	items := make([]MemberResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toResponse(row))
	}
	return items, nil
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (MemberResponse, error) {
	row, err := r.queries.CreateOrganizationMember(ctx, sqlcgen.CreateOrganizationMemberParams{
		ID:             newUUID(),
		OrganizationID: parseUUID(params.OrganizationID),
		UserID:         parseUUID(params.UserID),
		Role:           params.Role,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return MemberResponse{}, ErrMemberConflict
		}
		if isForeignKeyViolation(err) {
			return MemberResponse{}, ErrOrganizationNotFound
		}
		return MemberResponse{}, fmt.Errorf("create organization member: %w", err)
	}

	return toCreateResponse(row), nil
}

func (r *Repository) GetByID(ctx context.Context, memberID string) (MemberResponse, error) {
	row, err := r.queries.GetOrganizationMemberByID(ctx, parseUUID(memberID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemberResponse{}, ErrMemberNotFound
		}
		return MemberResponse{}, fmt.Errorf("get organization member: %w", err)
	}
	return toGetResponse(row), nil
}

func (r *Repository) UpdateRole(ctx context.Context, params UpdateParams) (MemberResponse, error) {
	row, err := r.queries.UpdateOrganizationMemberRole(ctx, sqlcgen.UpdateOrganizationMemberRoleParams{
		ID:   parseUUID(params.ID),
		Role: params.Role,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemberResponse{}, ErrMemberNotFound
		}
		return MemberResponse{}, fmt.Errorf("update organization member role: %w", err)
	}
	return toUpdateResponse(row), nil
}

func (r *Repository) Delete(ctx context.Context, memberID string) error {
	affected, err := r.queries.DeleteOrganizationMember(ctx, parseUUID(memberID))
	if err != nil {
		return fmt.Errorf("delete organization member: %w", err)
	}
	if affected == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (r *Repository) CountOwners(ctx context.Context, organizationID string) (int64, error) {
	count, err := r.queries.CountOrganizationOwners(ctx, parseUUID(organizationID))
	if err != nil {
		return 0, fmt.Errorf("count organization owners: %w", err)
	}
	return count, nil
}

func toCreateResponse(row sqlcgen.OrganizationMember) MemberResponse {
	return MemberResponse{
		ID:             uuidString(row.ID),
		OrganizationID: uuidString(row.OrganizationID),
		UserID:         uuidString(row.UserID),
		Role:           row.Role,
	}
}

func toGetResponse(row sqlcgen.OrganizationMember) MemberResponse {
	return MemberResponse{
		ID:             uuidString(row.ID),
		OrganizationID: uuidString(row.OrganizationID),
		UserID:         uuidString(row.UserID),
		Role:           row.Role,
	}
}

func toUpdateResponse(row sqlcgen.OrganizationMember) MemberResponse {
	return MemberResponse{
		ID:             uuidString(row.ID),
		OrganizationID: uuidString(row.OrganizationID),
		UserID:         uuidString(row.UserID),
		Role:           row.Role,
	}
}

func toResponse(row sqlcgen.OrganizationMember) MemberResponse {
	return MemberResponse{
		ID:             uuidString(row.ID),
		OrganizationID: uuidString(row.OrganizationID),
		UserID:         uuidString(row.UserID),
		Role:           row.Role,
	}
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func uuidString(value pgtype.UUID) string {
	return value.String()
}

func newUUID() pgtype.UUID {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	return pgtype.UUID{Bytes: bytes, Valid: true}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
