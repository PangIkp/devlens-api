package syncjob

import (
	"context"
	"log/slog"
	"sync"
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
	logger      *slog.Logger
	store       pendingJobStore
	service     *Service
	interval    time.Duration
	batchSize   int
	concurrency int
	jobTimeout  time.Duration
	metrics     *observability.Metrics
}

func NewWorker(logger *slog.Logger, store pendingJobStore, service *Service, interval time.Duration, batchSize int, concurrency int, jobTimeout time.Duration, metrics *observability.Metrics) *Worker {
	if batchSize <= 0 {
		batchSize = 10
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if jobTimeout <= 0 {
		jobTimeout = 5 * time.Minute
	}
	return &Worker{
		logger:      logger,
		store:       store,
		service:     service,
		interval:    interval,
		batchSize:   batchSize,
		concurrency: concurrency,
		jobTimeout:  jobTimeout,
		metrics:     metrics,
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

	ids, err := w.store.ListPendingIDs(ctx, w.batchSize)
	if err != nil {
		if w.metrics != nil {
			w.metrics.RecordWorkerIteration("syncjob", "error")
		}
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(jobID string) {
			defer wg.Done()
			defer func() { <-sem }()

			jobStarted := time.Now()
			jobCtx, cancel := context.WithTimeout(ctx, w.jobTimeout)
			defer cancel()

			job, err := w.service.ProcessPending(jobCtx, jobID)
			if err != nil {
				w.logger.Error("process pending sync job failed", "sync_job_id", jobID, "error", err)
				if w.metrics != nil {
					w.metrics.RecordWorkerJob("syncjob", "error", time.Since(jobStarted))
				}
				return
			}
			if w.metrics != nil {
				w.metrics.RecordWorkerJob("syncjob", job.Status, time.Since(jobStarted))
			}
			w.logger.Info("processed pending sync job", "sync_job_id", jobID, "status", job.Status)
		}(id)
	}
	wg.Wait()

	if w.metrics != nil {
		w.metrics.RecordWorkerIteration("syncjob", "ok")
	}
	span.SetAttributes(
		attribute.Int("devlens.sync.pending_jobs", len(ids)),
		attribute.Int("devlens.sync.worker_batch_size", w.batchSize),
		attribute.Int("devlens.sync.worker_concurrency", w.concurrency),
		attribute.Int64("devlens.sync.iteration_ms", time.Since(started).Milliseconds()),
	)
	span.SetStatus(codes.Ok, "")
	return nil
}
