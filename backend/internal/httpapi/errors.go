package httpapi

import (
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
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

func WriteError(w http.ResponseWriter, r *http.Request, status int, err Error) {
	if err.RequestID == "" {
		err.RequestID = chimiddleware.GetReqID(r.Context())
	}

	writeJSON(w, status, ErrorResponse{Error: err})
}
