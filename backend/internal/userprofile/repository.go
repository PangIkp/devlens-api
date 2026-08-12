package userprofile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrUserNotFound = errors.New("user not found")

type Repository struct {
	db *postgres.DB
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(ctx context.Context, id string) (Response, error) {
	query := `
SELECT id::text, email, name, avatar_url, created_at
FROM users
WHERE id = $1`

	var response Response
	var name, avatarURL pgtype.Text
	var createdAt time.Time
	err := r.db.Pool().QueryRow(ctx, query, parseUUID(id)).Scan(
		&response.ID,
		&response.Email,
		&name,
		&avatarURL,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Response{}, ErrUserNotFound
		}
		return Response{}, fmt.Errorf("get user: %w", err)
	}

	response.Name = optionalText(name)
	response.AvatarURL = optionalText(avatarURL)
	response.CreatedAt = createdAt.UTC()
	return response, nil
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}
