package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/PangIkp/devlens/backend/internal/auditlog"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/githubwebhook"
	"github.com/go-chi/chi/v5"
)

type GitHubWebhookService interface {
	Handle(rctx context.Context, req githubwebhook.HandleRequest) (githubwebhook.HandleResult, error)
	Retry(rctx context.Context, deliveryID string) (githubwebhook.HandleResult, error)
}

type GitHubWebhookHandler struct {
	service    GitHubWebhookService
	authorizer AuthorizationService
	audit      AuditLogger
}

func NewGitHubWebhookHandler(service GitHubWebhookService, deps ...any) *GitHubWebhookHandler {
	authz, auditLogger := resolveHandlerDeps(deps)
	return &GitHubWebhookHandler{service: service, authorizer: authz, audit: auditLogger}
}

func (h *GitHubWebhookHandler) RegisterRoutes(r chi.Router) {
	r.Post("/github/webhook", h.handle)
	r.Post("/webhooks/github", h.handle)
	if h.authorizer == nil {
		r.Post("/github/webhook-deliveries/{deliveryId}/retry", h.retry)
		return
	}

	r.With(requireWebhookDeliveryRoles(h.authorizer, authorization.RoleAdmin, authorization.RoleOwner)).Post("/github/webhook-deliveries/{deliveryId}/retry", h.retry)
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

func (h *GitHubWebhookHandler) retry(w http.ResponseWriter, r *http.Request) {
	deliveryID := chi.URLParam(r, "deliveryId")

	result, svcErr := h.service.Retry(r.Context(), deliveryID)
	if svcErr != nil {
		writeGitHubWebhookError(w, r, svcErr)
		return
	}
	userID, _ := currentUserID(r)
	recordAudit(r.Context(), h.audit, auditlog.Entry{
		ActorUserID:  stringRef(userID),
		Action:       "github_webhook.retry",
		ResourceType: "webhook_delivery",
		Metadata: map[string]any{
			"deliveryId": deliveryID,
			"eventType":  result.EventType,
		},
	})

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
		case errors.Is(err, githubwebhook.ErrDeliveryNotFound):
			WriteError(w, r, http.StatusNotFound, NewNotFoundError("GitHub webhook delivery not found"))
		case errors.Is(err, githubwebhook.ErrRetryNotAllowed):
			WriteError(w, r, http.StatusConflict, NewConflictError("Only failed webhook deliveries can be retried"))
		default:
			WriteError(w, r, http.StatusInternalServerError, NewInternalError())
		}
	}
}
