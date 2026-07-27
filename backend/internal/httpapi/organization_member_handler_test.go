package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PangIkp/devlens/backend/internal/organizationmember"
	"github.com/go-chi/chi/v5"
)

type stubOrganizationMemberService struct {
	listFn   func(context.Context, string) ([]organizationmember.MemberResponse, error)
	createFn func(context.Context, string, organizationmember.CreateOrganizationMemberRequest) (organizationmember.MemberResponse, error)
	updateFn func(context.Context, string, string, organizationmember.UpdateOrganizationMemberRequest) (organizationmember.MemberResponse, error)
	deleteFn func(context.Context, string, string) error
}

func (s stubOrganizationMemberService) List(ctx context.Context, organizationID string) ([]organizationmember.MemberResponse, error) {
	return s.listFn(ctx, organizationID)
}

func (s stubOrganizationMemberService) Create(ctx context.Context, organizationID string, req organizationmember.CreateOrganizationMemberRequest) (organizationmember.MemberResponse, error) {
	return s.createFn(ctx, organizationID, req)
}

func (s stubOrganizationMemberService) Update(ctx context.Context, organizationID string, memberID string, req organizationmember.UpdateOrganizationMemberRequest) (organizationmember.MemberResponse, error) {
	return s.updateFn(ctx, organizationID, memberID, req)
}

func (s stubOrganizationMemberService) Delete(ctx context.Context, organizationID string, memberID string) error {
	return s.deleteFn(ctx, organizationID, memberID)
}

func TestCreateOrganizationMemberHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationMemberHandler(stubOrganizationMemberService{
		createFn: func(_ context.Context, organizationID string, req organizationmember.CreateOrganizationMemberRequest) (organizationmember.MemberResponse, error) {
			if organizationID != "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d" {
				t.Fatalf("unexpected organization id %q", organizationID)
			}
			if req.UserID != "d18e6bc5-f4e9-4f27-8eb8-634becf5092e" || req.Role != "member" {
				t.Fatalf("unexpected request %+v", req)
			}
			return organizationmember.MemberResponse{
				ID:             "5fb3c674-6992-4ba9-a227-c1c66517e3f6",
				OrganizationID: organizationID,
				UserID:         req.UserID,
				Role:           req.Role,
			}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/members", strings.NewReader(`{"userId":"d18e6bc5-f4e9-4f27-8eb8-634becf5092e","role":"member"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestCreateOrganizationMemberHandlerDuplicate(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationMemberHandler(stubOrganizationMemberService{
		createFn: func(context.Context, string, organizationmember.CreateOrganizationMemberRequest) (organizationmember.MemberResponse, error) {
			return organizationmember.MemberResponse{}, organizationmember.ErrMemberConflict
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/members", strings.NewReader(`{"userId":"d18e6bc5-f4e9-4f27-8eb8-634becf5092e","role":"member"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestCreateOrganizationMemberHandlerInvalidRole(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationMemberHandler(stubOrganizationMemberService{
		createFn: func(context.Context, string, organizationmember.CreateOrganizationMemberRequest) (organizationmember.MemberResponse, error) {
			return organizationmember.MemberResponse{}, &organizationmember.ValidationError{
				Message: "request validation failed",
				Details: []organizationmember.ValidationIssue{
					{Field: "role", Message: "must be one of owner, admin, member"},
				},
			}
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/members", strings.NewReader(`{"userId":"d18e6bc5-f4e9-4f27-8eb8-634becf5092e","role":"viewer"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpdateOrganizationMemberHandlerFromAnotherOrganization(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationMemberHandler(stubOrganizationMemberService{
		updateFn: func(context.Context, string, string, organizationmember.UpdateOrganizationMemberRequest) (organizationmember.MemberResponse, error) {
			return organizationmember.MemberResponse{}, organizationmember.ErrMemberNotFound
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPatch, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/members/5fb3c674-6992-4ba9-a227-c1c66517e3f6", strings.NewReader(`{"role":"admin"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteOrganizationMemberHandlerLastOwnerConflict(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationMemberHandler(stubOrganizationMemberService{
		deleteFn: func(context.Context, string, string) error {
			return organizationmember.ErrLastOwnerConflict
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodDelete, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/members/5fb3c674-6992-4ba9-a227-c1c66517e3f6", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestDeleteOrganizationMemberHandlerSuccess(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationMemberHandler(stubOrganizationMemberService{
		deleteFn: func(context.Context, string, string) error {
			return nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodDelete, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/members/5fb3c674-6992-4ba9-a227-c1c66517e3f6", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestListOrganizationMembersHandlerResponseShape(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	NewOrganizationMemberHandler(stubOrganizationMemberService{
		listFn: func(context.Context, string) ([]organizationmember.MemberResponse, error) {
			return []organizationmember.MemberResponse{
				{
					ID:             "5fb3c674-6992-4ba9-a227-c1c66517e3f6",
					OrganizationID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
					UserID:         "d18e6bc5-f4e9-4f27-8eb8-634becf5092e",
					Role:           "member",
				},
			}, nil
		},
	}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/organizations/8f1cd971-1fd9-4f4f-9f75-47f6ed14938d/members", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Data []organizationmember.MemberResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("expected 1 member, got %d", len(body.Data))
	}
}
