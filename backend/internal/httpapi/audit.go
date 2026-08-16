package httpapi

import (
	"context"

	"github.com/PangIkp/devlens/backend/internal/auditlog"
)

type AuditLogger interface {
	Record(context.Context, auditlog.Entry) error
}

func recordAudit(ctx context.Context, logger AuditLogger, entry auditlog.Entry) {
	if logger == nil {
		return
	}
	_ = logger.Record(ctx, entry)
}

func stringRef(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func resolveHandlerDeps(deps []any) (AuthorizationService, AuditLogger) {
	var authorizer AuthorizationService
	var auditLogger AuditLogger
	for _, dep := range deps {
		switch value := dep.(type) {
		case AuthorizationService:
			authorizer = value
		case AuditLogger:
			auditLogger = value
		}
	}
	return authorizer, auditLogger
}
