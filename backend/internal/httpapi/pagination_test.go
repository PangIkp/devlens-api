package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParsePaginationDefaults(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)

	params, err := ParsePagination(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if params.Page != DefaultPage {
		t.Fatalf("expected page %d, got %d", DefaultPage, params.Page)
	}

	if params.PageSize != DefaultPageSize {
		t.Fatalf("expected pageSize %d, got %d", DefaultPageSize, params.PageSize)
	}
}

func TestParsePaginationExplicitValues(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations?page=2&pageSize=50", nil)

	params, err := ParsePagination(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if params.Page != 2 || params.PageSize != 50 {
		t.Fatalf("expected page=2 pageSize=50, got %+v", params)
	}
}

func TestParsePaginationInvalidValue(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations?page=abc", nil)

	_, err := ParsePagination(req)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}

	if validationErr.Details[0].Field != "page" {
		t.Fatalf("expected field page, got %q", validationErr.Details[0].Field)
	}
}

func TestParsePaginationTooSmall(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations?pageSize=0", nil)

	_, err := ParsePagination(req)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}

	if validationErr.Details[0].Field != "pageSize" {
		t.Fatalf("expected field pageSize, got %q", validationErr.Details[0].Field)
	}
}

func TestParsePaginationTooLarge(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations?pageSize=101", nil)

	_, err := ParsePagination(req)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}

	if validationErr.Details[0].Field != "pageSize" {
		t.Fatalf("expected field pageSize, got %q", validationErr.Details[0].Field)
	}
}

func TestNewPaginationMeta(t *testing.T) {
	t.Parallel()

	meta := NewPaginationMeta(2, 20, 42)
	if meta.TotalPages != 3 {
		t.Fatalf("expected totalPages 3, got %d", meta.TotalPages)
	}
}

func TestWritePage(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	WritePage(rec, http.StatusOK, []string{"a", "b"}, PaginationMeta{
		Page:       1,
		PageSize:   20,
		TotalItems: 2,
		TotalPages: 1,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body PageResponse[string]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if len(body.Data) != 2 {
		t.Fatalf("expected 2 data items, got %d", len(body.Data))
	}

	if body.Pagination.Page != 1 || body.Pagination.PageSize != 20 {
		t.Fatalf("unexpected pagination %+v", body.Pagination)
	}
}
