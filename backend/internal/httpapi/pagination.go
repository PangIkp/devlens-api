package httpapi

import (
	"net/http"
	"strconv"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

type PageParams struct {
	Page     int
	PageSize int
}

func ParsePagination(r *http.Request) (PageParams, error) {
	query := r.URL.Query()

	page, err := parsePositiveInt(query.Get("page"), DefaultPage, "page")
	if err != nil {
		return PageParams{}, err
	}

	pageSize, err := parsePositiveInt(query.Get("pageSize"), DefaultPageSize, "pageSize")
	if err != nil {
		return PageParams{}, err
	}

	if pageSize > MaxPageSize {
		return PageParams{}, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{
				FieldInvalid("pageSize", "must be less than or equal to 100"),
			},
		}
	}

	return PageParams{
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func NewPaginationMeta(page int, pageSize int, totalItems int) PaginationMeta {
	totalPages := 0
	if totalItems > 0 && pageSize > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}

	return PaginationMeta{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}
}

func parsePositiveInt(raw string, fallback int, field string) (int, error) {
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{
				FieldInvalid(field, "must be an integer"),
			},
		}
	}

	if value < 1 {
		return 0, &ValidationError{
			Message: "request validation failed",
			Details: []ValidationIssue{
				FieldInvalid(field, "must be greater than or equal to 1"),
			},
		}
	}

	return value, nil
}
