package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type store interface {
	CreateOrUpdateUser(context.Context, string, *string) (User, error)
	CreateSession(context.Context, createSessionParams) error
	GetSessionByAccessTokenHash(context.Context, string) (sessionRecord, error)
	GetSessionByRefreshTokenHash(context.Context, string) (sessionRecord, error)
	TouchSession(context.Context, string, time.Time) error
	RotateSession(context.Context, rotateSessionParams) error
	RevokeSession(context.Context, string, time.Time) error
}

type Service struct {
	repository      store
	now             func() time.Time
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewService(repository store, accessTokenTTL time.Duration, refreshTokenTTL time.Duration) *Service {
	if accessTokenTTL <= 0 {
		accessTokenTTL = 15 * time.Minute
	}
	if refreshTokenTTL <= 0 {
		refreshTokenTTL = 30 * 24 * time.Hour
	}
	return &Service{
		repository:      repository,
		now:             func() time.Time { return time.Now().UTC() },
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (s *Service) Login(ctx context.Context, req LoginRequest, metadata RequestMetadata) (SessionResponse, error) {
	if err := validateLoginRequest(req); err != nil {
		return SessionResponse{}, err
	}

	user, err := s.repository.CreateOrUpdateUser(ctx, req.Email, req.Name)
	if err != nil {
		return SessionResponse{}, err
	}

	now := s.now()
	accessToken, err := newToken()
	if err != nil {
		return SessionResponse{}, err
	}
	refreshToken, err := newToken()
	if err != nil {
		return SessionResponse{}, err
	}

	expiresAt := now.Add(s.accessTokenTTL)
	refreshExpiresAt := now.Add(s.refreshTokenTTL)
	if err := s.repository.CreateSession(ctx, createSessionParams{
		UserID:           user.ID,
		AccessTokenHash:  tokenHash(accessToken),
		RefreshTokenHash: tokenHash(refreshToken),
		ExpiresAt:        expiresAt,
		RefreshExpiresAt: refreshExpiresAt,
		UserAgent:        metadata.UserAgent,
		IPAddress:        metadata.IPAddress,
		Now:              now,
	}); err != nil {
		return SessionResponse{}, err
	}

	return SessionResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresAt:        expiresAt,
		RefreshExpiresAt: refreshExpiresAt,
		User:             user,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, req RefreshRequest) (SessionResponse, error) {
	token := strings.TrimSpace(req.RefreshToken)
	if token == "" {
		return SessionResponse{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{{Field: "refreshToken", Message: "is required"}},
		}
	}

	session, err := s.repository.GetSessionByRefreshTokenHash(ctx, tokenHash(token))
	if err != nil {
		return SessionResponse{}, err
	}
	now := s.now()
	if session.RevokedAt != nil || session.RefreshExpiresAt.Before(now) {
		return SessionResponse{}, ErrSessionExpired
	}

	accessToken, err := newToken()
	if err != nil {
		return SessionResponse{}, err
	}
	refreshToken, err := newToken()
	if err != nil {
		return SessionResponse{}, err
	}
	expiresAt := now.Add(s.accessTokenTTL)
	refreshExpiresAt := now.Add(s.refreshTokenTTL)

	if err := s.repository.RotateSession(ctx, rotateSessionParams{
		SessionID:           session.ID,
		OldRefreshTokenHash: session.RefreshTokenHash,
		AccessTokenHash:     tokenHash(accessToken),
		RefreshTokenHash:    tokenHash(refreshToken),
		ExpiresAt:           expiresAt,
		RefreshExpiresAt:    refreshExpiresAt,
		Now:                 now,
	}); err != nil {
		return SessionResponse{}, err
	}

	return SessionResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresAt:        expiresAt,
		RefreshExpiresAt: refreshExpiresAt,
		User:             session.User,
	}, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionNotFound
	}
	return s.repository.RevokeSession(ctx, sessionID, s.now())
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (SessionPrincipal, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return SessionPrincipal{}, ErrSessionNotFound
	}

	session, err := s.repository.GetSessionByAccessTokenHash(ctx, tokenHash(token))
	if err != nil {
		return SessionPrincipal{}, err
	}
	now := s.now()
	if session.RevokedAt != nil || session.ExpiresAt.Before(now) {
		return SessionPrincipal{}, ErrSessionExpired
	}
	if err := s.repository.TouchSession(ctx, session.ID, now); err != nil {
		return SessionPrincipal{}, err
	}
	return SessionPrincipal{
		SessionID: session.ID,
		User:      session.User,
	}, nil
}

type RequestMetadata struct {
	UserAgent *string
	IPAddress *string
}

func RequestMetadataFromHTTP(r *http.Request) RequestMetadata {
	var userAgent *string
	if value := strings.TrimSpace(r.UserAgent()); value != "" {
		userAgent = &value
	}
	var ipAddress *string
	if value := strings.TrimSpace(r.RemoteAddr); value != "" {
		ipAddress = &value
	}
	return RequestMetadata{
		UserAgent: userAgent,
		IPAddress: ipAddress,
	}
}

func validateLoginRequest(req LoginRequest) error {
	var issues []ValidationIssue
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		issues = append(issues, ValidationIssue{Field: "email", Message: "is required"})
	} else if !emailPattern.MatchString(email) {
		issues = append(issues, ValidationIssue{Field: "email", Message: "must be a valid email address"})
	}
	if len(issues) > 0 {
		return &ValidationError{Message: "request validation failed", Details: issues}
	}
	return nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}
