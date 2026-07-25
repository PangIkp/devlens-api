package httpapi

import "net/http"

type DataResponse[T any] struct {
	Data T `json:"data"`
}

type PaginationMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

type PageResponse[T any] struct {
	Data       []T            `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}

func WriteData[T any](w http.ResponseWriter, status int, data T) {
	writeJSON(w, status, DataResponse[T]{Data: data})
}

func WritePage[T any](w http.ResponseWriter, status int, data []T, pagination PaginationMeta) {
	writeJSON(w, status, PageResponse[T]{
		Data:       data,
		Pagination: pagination,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncoder(w, payload)
}
