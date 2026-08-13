package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/auth"
	"github.com/go-chi/chi/v5"
)

type stubAuthService struct {
	loginFn   func(context.Context, auth.LoginRequest, auth.RequestMetadata) (auth.SessionResponse, error)
	refreshFn func(context.Context, auth.RefreshRequest) (auth.SessionResponse, error)
	logoutFn  func(context.Context, string) error
}

func (s stubAuthService) Login(ctx context.Context, req auth.LoginRequest, metadata auth.RequestMetadata) (auth.SessionResponse, error) {
	return s.loginFn(ctx, req, metadata)
}

func (s stubAuthService) Refresh(ctx context.Context, req auth.RefreshRequest) (auth.SessionResponse, error) {
	return s.refreshFn(ctx, req)
}

func (s stubAuthService) Logout(ctx context.Context, sessionID string) error {
	return s.logoutFn(ctx, sessionID)
}

func TestAuthHandlerLogin(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(stubAuthService{
		loginFn: func(_ context.Context, req auth.LoginRequest, metadata auth.RequestMetadata) (auth.SessionResponse, error) {
			if req.Email != "devlens@example.com" {
				t.Fatalf("unexpected email %q", req.Email)
			}
			if req.Name == nil || *req.Name != "DevLens" {
				t.Fatalf("unexpected name %+v", req.Name)
			}
			if metadata.UserAgent == nil || *metadata.UserAgent == "" {
				t.Fatal("expected user agent metadata")
			}
			return auth.SessionResponse{
				AccessToken:      "access-token",
				RefreshToken:     "refresh-token",
				TokenType:        "Bearer",
				ExpiresAt:        time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
				RefreshExpiresAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
				User: auth.User{
					ID:        "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
					Email:     "devlens@example.com",
					CreatedAt: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
				},
			}, nil
		},
		refreshFn: func(context.Context, auth.RefreshRequest) (auth.SessionResponse, error) {
			t.Fatal("refresh should not be called")
			return auth.SessionResponse{}, nil
		},
		logoutFn: func(context.Context, string) error {
			t.Fatal("logout should not be called")
			return nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"devlens@example.com","name":"DevLens"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "devlens-test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Data auth.SessionResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Data.AccessToken != "access-token" {
		t.Fatalf("unexpected access token %q", body.Data.AccessToken)
	}
}

func TestAuthHandlerRefreshUnauthorized(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(stubAuthService{
		loginFn: func(context.Context, auth.LoginRequest, auth.RequestMetadata) (auth.SessionResponse, error) {
			t.Fatal("login should not be called")
			return auth.SessionResponse{}, nil
		},
		refreshFn: func(context.Context, auth.RefreshRequest) (auth.SessionResponse, error) {
			return auth.SessionResponse{}, auth.ErrSessionExpired
		},
		logoutFn: func(context.Context, string) error {
			t.Fatal("logout should not be called")
			return nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refreshToken":"expired"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthHandlerLogout(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(stubAuthService{
		loginFn: func(context.Context, auth.LoginRequest, auth.RequestMetadata) (auth.SessionResponse, error) {
			t.Fatal("login should not be called")
			return auth.SessionResponse{}, nil
		},
		refreshFn: func(context.Context, auth.RefreshRequest) (auth.SessionResponse, error) {
			t.Fatal("refresh should not be called")
			return auth.SessionResponse{}, nil
		},
		logoutFn: func(_ context.Context, sessionID string) error {
			if sessionID != "session-123" {
				t.Fatalf("unexpected session id %q", sessionID)
			}
			return nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.SessionPrincipal{SessionID: "session-123"}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestAuthHandlerLoginValidationError(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(stubAuthService{
		loginFn: func(context.Context, auth.LoginRequest, auth.RequestMetadata) (auth.SessionResponse, error) {
			return auth.SessionResponse{}, &auth.ValidationError{
				Message: "request validation failed",
				Details: []auth.ValidationIssue{{Field: "email", Message: "is required"}},
			}
		},
		refreshFn: func(context.Context, auth.RefreshRequest) (auth.SessionResponse, error) {
			t.Fatal("refresh should not be called")
			return auth.SessionResponse{}, nil
		},
		logoutFn: func(context.Context, string) error {
			t.Fatal("logout should not be called")
			return nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Error.Code != ErrorCodeValidation {
		t.Fatalf("expected %s, got %s", ErrorCodeValidation, body.Error.Code)
	}
}

func TestAuthHandlerLogoutUnauthorizedWithoutPrincipal(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(stubAuthService{
		loginFn: func(context.Context, auth.LoginRequest, auth.RequestMetadata) (auth.SessionResponse, error) {
			t.Fatal("login should not be called")
			return auth.SessionResponse{}, nil
		},
		refreshFn: func(context.Context, auth.RefreshRequest) (auth.SessionResponse, error) {
			t.Fatal("refresh should not be called")
			return auth.SessionResponse{}, nil
		},
		logoutFn: func(context.Context, string) error {
			t.Fatal("logout should not be called")
			return nil
		},
	})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWriteAuthErrorInternal(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rec := httptest.NewRecorder()

	writeAuthError(rec, req, errors.New("boom"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
