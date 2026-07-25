package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const (
	ErrorCodeBadRequest       = "BAD_REQUEST"
	ErrorCodeValidation       = "VALIDATION_ERROR"
	ErrorCodeUnauthorized     = "UNAUTHORIZED"
	ErrorCodeForbidden        = "FORBIDDEN"
	ErrorCodeNotFound         = "NOT_FOUND"
	ErrorCodeConflict         = "CONFLICT"
	ErrorCodeInternal         = "INTERNAL_ERROR"
	ErrorCodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
)

type Error struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"requestId,omitempty"`
	Details   []ValidationIssue `json:"details,omitempty"`
}

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error Error `json:"error"`
}

func NewBadRequestError(message string) Error {
	return Error{
		Code:    ErrorCodeBadRequest,
		Message: message,
	}
}

func NewValidationError(message string, details ...ValidationIssue) Error {
	return Error{
		Code:    ErrorCodeValidation,
		Message: message,
		Details: details,
	}
}

func NewUnauthorizedError(message string) Error {
	return Error{
		Code:    ErrorCodeUnauthorized,
		Message: message,
	}
}

func NewForbiddenError(message string) Error {
	return Error{
		Code:    ErrorCodeForbidden,
		Message: message,
	}
}

func NewNotFoundError(message string) Error {
	return Error{
		Code:    ErrorCodeNotFound,
		Message: message,
	}
}

func NewConflictError(message string) Error {
	return Error{
		Code:    ErrorCodeConflict,
		Message: message,
	}
}

func NewInternalError() Error {
	return Error{
		Code:    ErrorCodeInternal,
		Message: "Internal server error",
	}
}

func FieldRequired(field string) ValidationIssue {
	return ValidationIssue{
		Field:   field,
		Message: "is required",
	}
}

func FieldInvalid(field string, message string) ValidationIssue {
	return ValidationIssue{
		Field:   field,
		Message: message,
	}
}

func DecodeError(err error) Error {
	switch {
	case err == nil:
		return Error{}
	case errors.Is(err, io.EOF):
		return NewBadRequestError("Request body is required")
	default:
		var syntaxErr *SyntaxError
		if errors.As(err, &syntaxErr) {
			return NewBadRequestError(syntaxErr.Error())
		}

		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			return NewValidationError(validationErr.Message, validationErr.Details...)
		}

		return NewBadRequestError("Invalid request body")
	}
}

type ValidationError struct {
	Message string
	Details []ValidationIssue
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}

	return e.Message
}

type SyntaxError struct {
	Message string
}

func (e *SyntaxError) Error() string {
	if e == nil {
		return ""
	}

	return e.Message
}

func invalidJSONFieldError(field string) error {
	return &ValidationError{
		Message: "request validation failed",
		Details: []ValidationIssue{
			FieldInvalid(field, "is not allowed"),
		},
	}
}

func invalidJSONTypeError(field string) error {
	return &ValidationError{
		Message: "request validation failed",
		Details: []ValidationIssue{
			FieldInvalid(field, "has invalid type"),
		},
	}
}

func malformedJSONError(offset int64) error {
	return &SyntaxError{
		Message: fmt.Sprintf("Request body contains malformed JSON at position %d", offset),
	}
}

func multipleJSONValuesError() error {
	return &SyntaxError{
		Message: "Request body must contain only one JSON object",
	}
}

func bodyTooLargeError(limit int64) error {
	return &SyntaxError{
		Message: fmt.Sprintf("Request body must not be larger than %d bytes", limit),
	}
}

func emptyJSONFieldName(name string) string {
	return strings.TrimSpace(name)
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, err Error) {
	if err.RequestID == "" {
		err.RequestID = chimiddleware.GetReqID(r.Context())
	}

	writeJSON(w, status, ErrorResponse{Error: err})
}
