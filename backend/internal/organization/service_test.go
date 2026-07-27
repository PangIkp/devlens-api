package organization

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubRepository struct {
	createFn func(context.Context, CreateParams) (OrganizationResponse, error)
	getFn    func(context.Context, string) (OrganizationResponse, error)
	listFn   func(context.Context, ListParams) (ListResult, error)
	updateFn func(context.Context, UpdateParams) (OrganizationResponse, error)
	deleteFn func(context.Context, string) error
}

func (s stubRepository) Create(ctx context.Context, params CreateParams) (OrganizationResponse, error) {
	return s.createFn(ctx, params)
}

func (s stubRepository) GetByID(ctx context.Context, id string) (OrganizationResponse, error) {
	return s.getFn(ctx, id)
}

func (s stubRepository) List(ctx context.Context, params ListParams) (ListResult, error) {
	return s.listFn(ctx, params)
}

func (s stubRepository) Update(ctx context.Context, params UpdateParams) (OrganizationResponse, error) {
	return s.updateFn(ctx, params)
}

func (s stubRepository) SoftDelete(ctx context.Context, id string) error {
	return s.deleteFn(ctx, id)
}

func TestServiceCreateSuccess(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		createFn: func(_ context.Context, params CreateParams) (OrganizationResponse, error) {
			if params.GithubID != 123 || params.Slug != "devlens" || params.Name != "DevLens" {
				t.Fatalf("unexpected params %+v", params)
			}

			return OrganizationResponse{
				ID:        "org-1",
				GithubID:  123,
				Slug:      "devlens",
				Name:      "DevLens",
				CreatedAt: time.Now().UTC(),
			}, nil
		},
	})

	_, err := svc.Create(context.Background(), CreateOrganizationRequest{
		GithubID: 123,
		Slug:     "devlens",
		Name:     " DevLens ",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestServiceCreateValidationError(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		createFn: func(_ context.Context, _ CreateParams) (OrganizationResponse, error) {
			t.Fatal("repository should not be called")
			return OrganizationResponse{}, nil
		},
	})

	_, err := svc.Create(context.Background(), CreateOrganizationRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServiceGetByIDPassThrough(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		getFn: func(_ context.Context, id string) (OrganizationResponse, error) {
			if id != "abc" {
				t.Fatalf("unexpected id %q", id)
			}
			return OrganizationResponse{ID: id}, nil
		},
	})

	result, err := svc.GetByID(context.Background(), "abc")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ID != "abc" {
		t.Fatalf("expected id abc, got %q", result.ID)
	}
}

func TestServiceListPassThrough(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		listFn: func(_ context.Context, params ListParams) (ListResult, error) {
			if params.Page != 2 || params.PageSize != 10 {
				t.Fatalf("unexpected params %+v", params)
			}
			return ListResult{TotalItems: 1}, nil
		},
	})

	result, err := svc.List(context.Background(), ListParams{Page: 2, PageSize: 10})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.TotalItems != 1 {
		t.Fatalf("expected totalItems 1, got %d", result.TotalItems)
	}
}

func TestServiceCreateConflictPassThrough(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		createFn: func(_ context.Context, _ CreateParams) (OrganizationResponse, error) {
			return OrganizationResponse{}, ErrOrganizationConflict
		},
	})

	_, err := svc.Create(context.Background(), CreateOrganizationRequest{
		GithubID: 1,
		Slug:     "devlens",
		Name:     "DevLens",
	})
	if !errors.Is(err, ErrOrganizationConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestServiceUpdateSuccess(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		getFn: func(_ context.Context, id string) (OrganizationResponse, error) {
			return OrganizationResponse{
				ID:       id,
				GithubID: 123,
				Slug:     "devlens",
				Name:     "DevLens",
			}, nil
		},
		updateFn: func(_ context.Context, params UpdateParams) (OrganizationResponse, error) {
			if params.Slug != "devlens-platform" || params.Name != "DevLens" {
				t.Fatalf("unexpected params %+v", params)
			}
			return OrganizationResponse{ID: params.ID, Slug: params.Slug, Name: params.Name}, nil
		},
	})

	slug := "devlens-platform"
	_, err := svc.Update(context.Background(), "org-1", UpdateOrganizationRequest{Slug: &slug})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestServiceInvalidUpdate(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		getFn: func(_ context.Context, id string) (OrganizationResponse, error) {
			return OrganizationResponse{
				ID:       id,
				GithubID: 123,
				Slug:     "devlens",
				Name:     "DevLens",
			}, nil
		},
		updateFn: func(_ context.Context, _ UpdateParams) (OrganizationResponse, error) {
			t.Fatal("repository update should not be called")
			return OrganizationResponse{}, nil
		},
	})

	_, err := svc.Update(context.Background(), "org-1", UpdateOrganizationRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServiceSoftDelete(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		deleteFn: func(_ context.Context, id string) error {
			if id != "org-1" {
				t.Fatalf("unexpected id %q", id)
			}
			return nil
		},
	})

	if err := svc.SoftDelete(context.Background(), "org-1"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestServiceCreateSlugValidationError(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		createFn: func(_ context.Context, _ CreateParams) (OrganizationResponse, error) {
			t.Fatal("repository should not be called")
			return OrganizationResponse{}, nil
		},
	})

	_, err := svc.Create(context.Background(), CreateOrganizationRequest{
		GithubID: 1,
		Slug:     "DevLens",
		Name:     "DevLens",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
