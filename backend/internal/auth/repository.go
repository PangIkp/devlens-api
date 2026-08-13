package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

type Repository struct {
	db *postgres.DB
}

type userRecord struct {
	ID        string
	Email     string
	Name      *string
	AvatarURL *string
	CreatedAt time.Time
}

type sessionRecord struct {
	ID               string
	UserID           string
	AccessTokenHash  string
	RefreshTokenHash string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	RevokedAt        *time.Time
	UserAgent        *string
	IPAddress        *string
	CreatedAt        time.Time
	UpdatedAt        *time.Time
	User             User
}

type createSessionParams struct {
	UserID           string
	AccessTokenHash  string
	RefreshTokenHash string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	UserAgent        *string
	IPAddress        *string
	Now              time.Time
}

type rotateSessionParams struct {
	SessionID        string
	AccessTokenHash  string
	RefreshTokenHash string
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	Now              time.Time
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateOrUpdateUser(ctx context.Context, email string, name *string) (User, error) {
	query := `
INSERT INTO users (id, email, name, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE SET
    name = CASE
        WHEN EXCLUDED.name IS NULL OR EXCLUDED.name = '' THEN users.name
        ELSE EXCLUDED.name
    END
RETURNING id::text, email, name, avatar_url, created_at`

	now := time.Now().UTC()
	var id, returnedEmail string
	var returnedName, avatarURL pgtype.Text
	var createdAt time.Time
	err := r.db.Pool().QueryRow(ctx, query, newUUID(), strings.TrimSpace(strings.ToLower(email)), nullableText(name), now).Scan(
		&id,
		&returnedEmail,
		&returnedName,
		&avatarURL,
		&createdAt,
	)
	if err != nil {
		return User{}, fmt.Errorf("create or update user: %w", err)
	}
	return User{
		ID:        id,
		Email:     returnedEmail,
		Name:      optionalText(returnedName),
		AvatarURL: optionalText(avatarURL),
		CreatedAt: createdAt.UTC(),
	}, nil
}

func (r *Repository) CreateSession(ctx context.Context, params createSessionParams) error {
	query := `
INSERT INTO user_sessions (
    id,
    user_id,
    access_token_hash,
    refresh_token_hash,
    expires_at,
    refresh_expires_at,
    last_used_at,
    user_agent,
    ip_address,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $7, $7)`

	_, err := r.db.Pool().Exec(ctx, query,
		newUUID(),
		parseUUID(params.UserID),
		params.AccessTokenHash,
		params.RefreshTokenHash,
		params.ExpiresAt.UTC(),
		params.RefreshExpiresAt.UTC(),
		params.Now.UTC(),
		nullableText(params.UserAgent),
		nullableText(params.IPAddress),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *Repository) GetSessionByAccessTokenHash(ctx context.Context, accessTokenHash string) (sessionRecord, error) {
	return r.getSession(ctx, `
SELECT s.id::text,
       s.user_id::text,
       s.access_token_hash,
       s.refresh_token_hash,
       s.expires_at,
       s.refresh_expires_at,
       s.revoked_at,
       s.user_agent,
       s.ip_address,
       s.created_at,
       s.updated_at,
       u.id::text,
       u.email,
       u.name,
       u.avatar_url,
       u.created_at
FROM user_sessions s
INNER JOIN users u ON u.id = s.user_id
WHERE s.access_token_hash = $1`, accessTokenHash)
}

func (r *Repository) GetSessionByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (sessionRecord, error) {
	return r.getSession(ctx, `
SELECT s.id::text,
       s.user_id::text,
       s.access_token_hash,
       s.refresh_token_hash,
       s.expires_at,
       s.refresh_expires_at,
       s.revoked_at,
       s.user_agent,
       s.ip_address,
       s.created_at,
       s.updated_at,
       u.id::text,
       u.email,
       u.name,
       u.avatar_url,
       u.created_at
FROM user_sessions s
INNER JOIN users u ON u.id = s.user_id
WHERE s.refresh_token_hash = $1`, refreshTokenHash)
}

func (r *Repository) TouchSession(ctx context.Context, sessionID string, at time.Time) error {
	_, err := r.db.Pool().Exec(ctx, `UPDATE user_sessions SET last_used_at = $2, updated_at = $2 WHERE id = $1`, parseUUID(sessionID), at.UTC())
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (r *Repository) RotateSession(ctx context.Context, params rotateSessionParams) error {
	query := `
UPDATE user_sessions
SET access_token_hash = $2,
    refresh_token_hash = $3,
    expires_at = $4,
    refresh_expires_at = $5,
    last_used_at = $6,
    updated_at = $6
WHERE id = $1
  AND revoked_at IS NULL`

	commandTag, err := r.db.Pool().Exec(ctx, query,
		parseUUID(params.SessionID),
		params.AccessTokenHash,
		params.RefreshTokenHash,
		params.ExpiresAt.UTC(),
		params.RefreshExpiresAt.UTC(),
		params.Now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("rotate session: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID string, at time.Time) error {
	query := `UPDATE user_sessions SET revoked_at = $2, updated_at = $2 WHERE id = $1 AND revoked_at IS NULL`
	commandTag, err := r.db.Pool().Exec(ctx, query, parseUUID(sessionID), at.UTC())
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (r *Repository) getSession(ctx context.Context, query string, tokenHash string) (sessionRecord, error) {
	var session sessionRecord
	var revokedAt, updatedAt pgtype.Timestamptz
	var userName, avatarURL pgtype.Text
	var userCreatedAt time.Time
	var userAgent, ipAddress pgtype.Text
	err := r.db.Pool().QueryRow(ctx, query, tokenHash).Scan(
		&session.ID,
		&session.UserID,
		&session.AccessTokenHash,
		&session.RefreshTokenHash,
		&session.ExpiresAt,
		&session.RefreshExpiresAt,
		&revokedAt,
		&userAgent,
		&ipAddress,
		&session.CreatedAt,
		&updatedAt,
		&session.User.ID,
		&session.User.Email,
		&userName,
		&avatarURL,
		&userCreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sessionRecord{}, ErrSessionNotFound
		}
		return sessionRecord{}, fmt.Errorf("get session: %w", err)
	}
	session.ExpiresAt = session.ExpiresAt.UTC()
	session.RefreshExpiresAt = session.RefreshExpiresAt.UTC()
	session.CreatedAt = session.CreatedAt.UTC()
	session.RevokedAt = optionalTime(revokedAt)
	session.UpdatedAt = optionalTime(updatedAt)
	session.UserAgent = optionalText(userAgent)
	session.IPAddress = optionalText(ipAddress)
	session.User.Name = optionalText(userName)
	session.User.AvatarURL = optionalText(avatarURL)
	session.User.CreatedAt = userCreatedAt.UTC()
	return session, nil
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

func nullableText(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}
