package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/githubwebhook"
	"github.com/go-chi/chi/v5"
)

type GitHubWebhookService interface {
	Handle(rctx context.Context, req githubwebhook.HandleRequest) (githubwebhook.HandleResult, error)
}

type GitHubWebhookHandler struct {
	service GitHubWebhookService
}

func NewGitHubWebhookHandler(service GitHubWebhookService) *GitHubWebhookHandler {
	return &GitHubWebhookHandler{service: service}
}

func (h *GitHubWebhookHandler) RegisterRoutes(r chi.Router) {
	r.Post("/github/webhook", h.handle)
}

func (h *GitHubWebhookHandler) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, DecodeError(err))
		return
	}

	result, svcErr := h.service.Handle(r.Context(), githubwebhook.HandleRequest{
		DeliveryID: r.Header.Get("X-GitHub-Delivery"),
		EventType:  r.Header.Get("X-GitHub-Event"),
		Signature:  r.Header.Get("X-Hub-Signature-256"),
		Body:       body,
	})
	if svcErr != nil {
		writeGitHubWebhookError(w, r, svcErr)
		return
	}

	WriteData(w, http.StatusAccepted, result)
}

func writeGitHubWebhookError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case nil:
		return
	default:
		switch {
		case errors.Is(err, githubwebhook.ErrInvalidSignature):
			WriteError(w, r, http.StatusUnauthorized, Error{Code: "INVALID_WEBHOOK_SIGNATURE", Message: "Invalid webhook signature"})
		case errors.Is(err, githubwebhook.ErrMissingDelivery):
			WriteError(w, r, http.StatusBadRequest, Error{Code: "MISSING_WEBHOOK_DELIVERY_ID", Message: "Missing GitHub delivery ID"})
		case errors.Is(err, githubwebhook.ErrMissingEvent):
			WriteError(w, r, http.StatusBadRequest, Error{Code: "MISSING_WEBHOOK_EVENT", Message: "Missing GitHub event type"})
		default:
			WriteError(w, r, http.StatusInternalServerError, NewInternalError())
		}
	}
}
