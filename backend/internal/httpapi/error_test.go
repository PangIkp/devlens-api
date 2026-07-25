package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONSuccess(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", strings.NewReader(`{"name":"DevLens"}`))

	var got payload
	if err := DecodeJSON(req, &got); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got.Name != "DevLens" {
		t.Fatalf("expected name DevLens, got %q", got.Name)
	}
}

func TestDecodeJSONUnknownField(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", strings.NewReader(`{"name":"DevLens","nickname":"devlens"}`))

	var got payload
	err := DecodeJSON(req, &got)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}

	if validationErr.Details[0].Field != "nickname" {
		t.Fatalf("expected field nickname, got %q", validationErr.Details[0].Field)
	}
}

func TestDecodeJSONInvalidType(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", strings.NewReader(`{"name":123}`))

	var got payload
	err := DecodeJSON(req, &got)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}

	if validationErr.Details[0].Field != "name" {
		t.Fatalf("expected field name, got %q", validationErr.Details[0].Field)
	}
}

func TestDecodeJSONEmptyBody(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", nil)

	var got payload
	err := DecodeJSON(req, &got)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	apiErr := DecodeError(err)
	if apiErr.Code != ErrorCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %q", apiErr.Code)
	}
}

func TestDecodeErrorValidationError(t *testing.T) {
	t.Parallel()

	err := DecodeError(&ValidationError{
		Message: "request validation failed",
		Details: []ValidationIssue{
			FieldRequired("name"),
		},
	})

	if err.Code != ErrorCodeValidation {
		t.Fatalf("expected VALIDATION_ERROR, got %q", err.Code)
	}

	if len(err.Details) != 1 || err.Details[0].Field != "name" {
		t.Fatalf("expected validation detail for name, got %+v", err.Details)
	}
}

func TestDecodeErrorSyntaxError(t *testing.T) {
	t.Parallel()

	err := DecodeError(&SyntaxError{Message: "Request body contains malformed JSON"})
	if err.Code != ErrorCodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %q", err.Code)
	}
}

func TestNewUnauthorizedError(t *testing.T) {
	t.Parallel()

	err := NewUnauthorizedError("Authentication required")
	if err.Code != ErrorCodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED, got %q", err.Code)
	}
}

func TestNewForbiddenError(t *testing.T) {
	t.Parallel()

	err := NewForbiddenError("Access denied")
	if err.Code != ErrorCodeForbidden {
		t.Fatalf("expected FORBIDDEN, got %q", err.Code)
	}
}
