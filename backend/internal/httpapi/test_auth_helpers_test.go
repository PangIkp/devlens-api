package httpapi

import (
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/auth"
)

func withAuthenticatedUser(r *http.Request) *http.Request {
	return r.WithContext(auth.WithPrincipal(r.Context(), auth.SessionPrincipal{
		SessionID: "session-test",
		User: auth.User{
			ID: "8f1cd971-1fd9-4f4f-9f75-47f6ed14938d",
		},
	}))
}
