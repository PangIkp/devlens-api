package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", "panic", fmt.Sprint(rec), "method", r.Method, "path", r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"error": map[string]any{
							"code":      "INTERNAL_SERVER_ERROR",
							"message":   "Internal server error",
							"requestId": chimiddleware.GetReqID(r.Context()),
						},
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
