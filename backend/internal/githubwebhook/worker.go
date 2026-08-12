package githubwebhook

import (
	"context"
	"log/slog"
	"time"
)

type retryProcessor interface {
	RetryFailedPending(context.Context, int) error
}

type Worker struct {
	logger   *slog.Logger
	service  retryProcessor
	interval time.Duration
}

func NewWorker(logger *slog.Logger, service retryProcessor, interval time.Duration) *Worker {
	return &Worker{
		logger:   logger,
		service:  service,
		interval: interval,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.service.RetryFailedPending(ctx, 10); err != nil {
			w.logger.Error("webhook retry worker iteration failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
