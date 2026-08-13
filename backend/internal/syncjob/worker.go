package syncjob

import (
	"context"
	"log/slog"
	"time"

	"github.com/PangIkp/devlens/backend/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type pendingJobStore interface {
	ListPendingIDs(context.Context, int) ([]string, error)
}

type Worker struct {
	logger   *slog.Logger
	store    pendingJobStore
	service  *Service
	interval time.Duration
	metrics  *observability.Metrics
}

func NewWorker(logger *slog.Logger, store pendingJobStore, service *Service, interval time.Duration, metrics *observability.Metrics) *Worker {
	return &Worker{
		logger:   logger,
		store:    store,
		service:  service,
		interval: interval,
		metrics:  metrics,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.processOnce(ctx); err != nil {
			w.logger.Error("sync worker iteration failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) error {
	started := time.Now()
	ctx, span := otel.Tracer("devlens/sync-worker").Start(ctx, "syncjob.process_once")
	defer span.End()

	ids, err := w.store.ListPendingIDs(ctx, 10)
	if err != nil {
		if w.metrics != nil {
			w.metrics.RecordWorkerIteration("syncjob", "error")
		}
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	for _, id := range ids {
		jobStarted := time.Now()
		job, err := w.service.ProcessPending(ctx, id)
		if err != nil {
			w.logger.Error("process pending sync job failed", "sync_job_id", id, "error", err)
			if w.metrics != nil {
				w.metrics.RecordWorkerJob("syncjob", "error", time.Since(jobStarted))
			}
			continue
		}
		if w.metrics != nil {
			w.metrics.RecordWorkerJob("syncjob", job.Status, time.Since(jobStarted))
		}
		w.logger.Info("processed pending sync job", "sync_job_id", id, "status", job.Status)
	}

	if w.metrics != nil {
		w.metrics.RecordWorkerIteration("syncjob", "ok")
	}
	span.SetAttributes(
		attribute.Int("devlens.sync.pending_jobs", len(ids)),
		attribute.Int64("devlens.sync.iteration_ms", time.Since(started).Milliseconds()),
	)
	span.SetStatus(codes.Ok, "")
	return nil
}
