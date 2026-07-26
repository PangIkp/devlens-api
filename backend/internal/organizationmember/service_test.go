package organizationmember

import (
	"context"
	"errors"
	"testing"
)

type stubRepository struct {
	ensureOrganizationExistsFn func(context.Context, string) error
	ensureUserExistsFn         func(context.Context, string) error
	listFn                     func(context.Context, string) ([]MemberResponse, error)
	createFn                   func(context.Context, CreateParams) (MemberResponse, error)
	getByIDFn                  func(context.Context, string) (MemberResponse, error)
	updateRoleFn               func(context.Context, UpdateParams) (MemberResponse, error)
	deleteFn                   func(context.Context, string) error
	countOwnersFn              func(context.Context, string) (int64, error)
}

func (s stubRepository) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	return s.ensureOrganizationExistsFn(ctx, organizationID)
}

func (s stubRepository) EnsureUserExists(ctx context.Context, userID string) error {
	return s.ensureUserExistsFn(ctx, userID)
}

func (s stubRepository) List(ctx context.Context, organizationID string) ([]MemberResponse, error) {
	return s.listFn(ctx, organizationID)
}

func (s stubRepository) Create(ctx context.Context, params CreateParams) (MemberResponse, error) {
	return s.createFn(ctx, params)
}

func (s stubRepository) GetByID(ctx context.Context, memberID string) (MemberResponse, error) {
	return s.getByIDFn(ctx, memberID)
}

func (s stubRepository) UpdateRole(ctx context.Context, params UpdateParams) (MemberResponse, error) {
	return s.updateRoleFn(ctx, params)
}

func (s stubRepository) Delete(ctx context.Context, memberID string) error {
	return s.deleteFn(ctx, memberID)
}

func (s stubRepository) CountOwners(ctx context.Context, organizationID string) (int64, error) {
	return s.countOwnersFn(ctx, organizationID)
}

func TestServiceCreateSuccess(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		ensureOrganizationExistsFn: func(_ context.Context, organizationID string) error {
			if organizationID != "org-1" {
				t.Fatalf("unexpected organization id %q", organizationID)
			}
			return nil
		},
		ensureUserExistsFn: func(_ context.Context, userID string) error {
			if userID != "user-1" {
				t.Fatalf("unexpected user id %q", userID)
			}
			return nil
		},
		createFn: func(_ context.Context, params CreateParams) (MemberResponse, error) {
			if params.OrganizationID != "org-1" || params.UserID != "user-1" || params.Role != "member" {
				t.Fatalf("unexpected params %+v", params)
			}
			return MemberResponse{ID: "member-1", OrganizationID: params.OrganizationID, UserID: params.UserID, Role: params.Role}, nil
		},
	})

	item, err := svc.Create(context.Background(), "org-1", CreateOrganizationMemberRequest{
		UserID: "user-1",
		Role:   "member",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if item.ID != "member-1" {
		t.Fatalf("expected member-1, got %q", item.ID)
	}
}

func TestServiceCreateDuplicateMember(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		ensureOrganizationExistsFn: func(context.Context, string) error { return nil },
		ensureUserExistsFn:         func(context.Context, string) error { return nil },
		createFn: func(_ context.Context, _ CreateParams) (MemberResponse, error) {
			return MemberResponse{}, ErrMemberConflict
		},
	})

	_, err := svc.Create(context.Background(), "org-1", CreateOrganizationMemberRequest{
		UserID: "user-1",
		Role:   "member",
	})
	if !errors.Is(err, ErrMemberConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestServiceCreateInvalidRole(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		ensureOrganizationExistsFn: func(context.Context, string) error {
			t.Fatal("organization lookup should not be called")
			return nil
		},
		ensureUserExistsFn: func(context.Context, string) error {
			t.Fatal("user lookup should not be called")
			return nil
		},
		createFn: func(context.Context, CreateParams) (MemberResponse, error) {
			t.Fatal("create should not be called")
			return MemberResponse{}, nil
		},
	})

	_, err := svc.Create(context.Background(), "org-1", CreateOrganizationMemberRequest{
		UserID: "user-1",
		Role:   "viewer",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestServiceUpdateMemberFromAnotherOrganization(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		ensureOrganizationExistsFn: func(context.Context, string) error { return nil },
		getByIDFn: func(_ context.Context, memberID string) (MemberResponse, error) {
			return MemberResponse{
				ID:             memberID,
				OrganizationID: "org-2",
				UserID:         "user-1",
				Role:           "member",
			}, nil
		},
		updateRoleFn: func(context.Context, UpdateParams) (MemberResponse, error) {
			t.Fatal("update should not be called")
			return MemberResponse{}, nil
		},
	})

	role := "admin"
	_, err := svc.Update(context.Background(), "org-1", "member-1", UpdateOrganizationMemberRequest{Role: &role})
	if !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestServiceDeleteLastOwnerConflict(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		ensureOrganizationExistsFn: func(context.Context, string) error { return nil },
		getByIDFn: func(_ context.Context, memberID string) (MemberResponse, error) {
			return MemberResponse{
				ID:             memberID,
				OrganizationID: "org-1",
				UserID:         "user-1",
				Role:           "owner",
			}, nil
		},
		countOwnersFn: func(_ context.Context, organizationID string) (int64, error) {
			if organizationID != "org-1" {
				t.Fatalf("unexpected organization id %q", organizationID)
			}
			return 1, nil
		},
		deleteFn: func(context.Context, string) error {
			t.Fatal("delete should not be called")
			return nil
		},
	})

	err := svc.Delete(context.Background(), "org-1", "member-1")
	if !errors.Is(err, ErrLastOwnerConflict) {
		t.Fatalf("expected last owner conflict, got %v", err)
	}
}

func TestServiceDeleteSuccess(t *testing.T) {
	t.Parallel()

	svc := NewService(stubRepository{
		ensureOrganizationExistsFn: func(context.Context, string) error { return nil },
		getByIDFn: func(_ context.Context, memberID string) (MemberResponse, error) {
			return MemberResponse{
				ID:             memberID,
				OrganizationID: "org-1",
				UserID:         "user-1",
				Role:           "admin",
			}, nil
		},
		deleteFn: func(_ context.Context, memberID string) error {
			if memberID != "member-1" {
				t.Fatalf("unexpected member id %q", memberID)
			}
			return nil
		},
		countOwnersFn: func(context.Context, string) (int64, error) {
			t.Fatal("count owners should not be called")
			return 0, nil
		},
	})

	if err := svc.Delete(context.Background(), "org-1", "member-1"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
