package githubwebhook

import (
	"context"
	"log/slog"
	"time"

	"github.com/PangIkp/devlens/backend/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type retryProcessor interface {
	RetryFailedPending(context.Context, int) error
}

type Worker struct {
	logger    *slog.Logger
	service   retryProcessor
	interval  time.Duration
	batchSize int
	metrics   *observability.Metrics
}

func NewWorker(logger *slog.Logger, service retryProcessor, interval time.Duration, batchSize int, metrics *observability.Metrics) *Worker {
	if batchSize <= 0 {
		batchSize = 10
	}
	return &Worker{
		logger:    logger,
		service:   service,
		interval:  interval,
		batchSize: batchSize,
		metrics:   metrics,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		started := time.Now()
		iterationCtx, span := otel.Tracer("devlens/webhook-worker").Start(ctx, "githubwebhook.retry_failed_pending")
		err := w.service.RetryFailedPending(iterationCtx, w.batchSize)
		if err != nil {
			w.logger.Error("webhook retry worker iteration failed", "error", err)
			if w.metrics != nil {
				w.metrics.RecordWorkerIteration("githubwebhook", "error")
				w.metrics.RecordWorkerJob("githubwebhook", "error", time.Since(started))
			}
			span.SetStatus(codes.Error, err.Error())
		} else {
			if w.metrics != nil {
				w.metrics.RecordWorkerIteration("githubwebhook", "ok")
				w.metrics.RecordWorkerJob("githubwebhook", "ok", time.Since(started))
			}
			span.SetStatus(codes.Ok, "")
		}
		span.SetAttributes(
			attribute.Int("devlens.webhook.retry_batch_size", w.batchSize),
			attribute.Int64("devlens.webhook.retry_iteration_ms", time.Since(started).Milliseconds()),
		)
		span.End()

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
