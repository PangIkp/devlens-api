package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

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

func WriteDataConditional[T any](w http.ResponseWriter, r *http.Request, status int, data T) {
	writeJSONConditional(w, r, status, DataResponse[T]{Data: data})
}

func WritePageConditional[T any](w http.ResponseWriter, r *http.Request, status int, data []T, pagination PaginationMeta) {
	writeJSONConditional(w, r, status, PageResponse[T]{
		Data:       data,
		Pagination: pagination,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncoder(w, payload)
}

func writeJSONConditional(w http.ResponseWriter, r *http.Request, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error: Error{
				Code:      ErrorCodeInternal,
				Message:   "Internal server error",
				RequestID: chimiddleware.GetReqID(r.Context()),
			},
		})
		return
	}

	etag := entityTag(body)
	if matchesEntityTag(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func entityTag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func matchesEntityTag(headerValue string, target string) bool {
	for _, raw := range strings.Split(headerValue, ",") {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}
		if candidate == "*" {
			return true
		}
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == target {
			return true
		}
	}
	return false
}
