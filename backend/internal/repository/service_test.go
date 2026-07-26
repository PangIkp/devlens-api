package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubStore struct {
	ensureOrganizationExistsFn func(context.Context, string) error
	createFn                   func(context.Context, CreateParams) (RepositoryResponse, error)
	getFn                      func(context.Context, string) (RepositoryResponse, error)
	listFn                     func(context.Context, ListParams) (ListResult, error)
	updateFn                   func(context.Context, UpdateParams) (RepositoryResponse, error)
}

func (s stubStore) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	return s.ensureOrganizationExistsFn(ctx, organizationID)
}

func (s stubStore) Create(ctx context.Context, params CreateParams) (RepositoryResponse, error) {
	return s.createFn(ctx, params)
}

func (s stubStore) GetByID(ctx context.Context, id string) (RepositoryResponse, error) {
	return s.getFn(ctx, id)
}

func (s stubStore) List(ctx context.Context, params ListParams) (ListResult, error) {
	return s.listFn(ctx, params)
}

func (s stubStore) Update(ctx context.Context, params UpdateParams) (RepositoryResponse, error) {
	return s.updateFn(ctx, params)
}

func TestServiceCreateSuccess(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		ensureOrganizationExistsFn: func(_ context.Context, organizationID string) error {
			if organizationID != "org-1" {
				t.Fatalf("unexpected organization id %q", organizationID)
			}
			return nil
		},
		createFn: func(_ context.Context, params CreateParams) (RepositoryResponse, error) {
			if params.GithubID != 42 || params.Name != "devlens-api" || params.FullName != "devlens-labs/devlens-api" {
				t.Fatalf("unexpected params %+v", params)
			}
			return RepositoryResponse{ID: "repo-1"}, nil
		},
	})

	_, err := svc.Create(context.Background(), "org-1", CreateRepositoryRequest{
		GithubID: 42,
		Name:     "devlens-api",
		FullName: "devlens-labs/devlens-api",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestServiceCreateValidationError(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		ensureOrganizationExistsFn: func(context.Context, string) error {
			t.Fatal("ensure organization should not be called")
			return nil
		},
		createFn: func(context.Context, CreateParams) (RepositoryResponse, error) {
			t.Fatal("create should not be called")
			return RepositoryResponse{}, nil
		},
	})

	_, err := svc.Create(context.Background(), "org-1", CreateRepositoryRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServiceListInvalidStatus(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		ensureOrganizationExistsFn: func(context.Context, string) error {
			t.Fatal("ensure organization should not be called")
			return nil
		},
		listFn: func(context.Context, ListParams) (ListResult, error) {
			t.Fatal("list should not be called")
			return ListResult{}, nil
		},
	})

	_, err := svc.List(context.Background(), ListParams{
		OrganizationID: "org-1",
		Page:           1,
		PageSize:       20,
		Status:         "disabled",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServiceUpdateEmptyBody(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		getFn: func(context.Context, string) (RepositoryResponse, error) {
			return RepositoryResponse{
				ID:             "repo-1",
				OrganizationID: "org-1",
				GithubID:       42,
				Name:           "devlens-api",
				FullName:       "devlens-labs/devlens-api",
				IsActive:       true,
				CreatedAt:      time.Now().UTC(),
			}, nil
		},
		updateFn: func(context.Context, UpdateParams) (RepositoryResponse, error) {
			t.Fatal("update should not be called")
			return RepositoryResponse{}, nil
		},
	})

	_, err := svc.Update(context.Background(), "repo-1", UpdateRepositoryRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServiceUpdateArchiveSuccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	svc := NewService(stubStore{
		getFn: func(context.Context, string) (RepositoryResponse, error) {
			return RepositoryResponse{
				ID:             "repo-1",
				OrganizationID: "org-1",
				GithubID:       42,
				Name:           "devlens-api",
				FullName:       "devlens-labs/devlens-api",
				IsActive:       true,
				CreatedAt:      now,
			}, nil
		},
		updateFn: func(_ context.Context, params UpdateParams) (RepositoryResponse, error) {
			if params.ArchivedAt == nil {
				t.Fatal("expected archivedAt to be set")
			}
			return RepositoryResponse{ID: params.ID, ArchivedAt: params.ArchivedAt}, nil
		},
	})
	svc.now = func() time.Time { return now }

	archived := true
	result, err := svc.Update(context.Background(), "repo-1", UpdateRepositoryRequest{Archived: &archived})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ArchivedAt == nil {
		t.Fatal("expected archivedAt in response")
	}
}

func TestServiceCreateConflictPassThrough(t *testing.T) {
	t.Parallel()

	svc := NewService(stubStore{
		ensureOrganizationExistsFn: func(context.Context, string) error { return nil },
		createFn: func(context.Context, CreateParams) (RepositoryResponse, error) {
			return RepositoryResponse{}, ErrRepositoryConflict
		},
	})

	_, err := svc.Create(context.Background(), "org-1", CreateRepositoryRequest{
		GithubID: 42,
		Name:     "devlens-api",
		FullName: "devlens-labs/devlens-api",
	})
	if !errors.Is(err, ErrRepositoryConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
