package syncjob

import (
	"context"
	"log/slog"
	"time"
)

type pendingJobStore interface {
	ListPendingIDs(context.Context, int) ([]string, error)
}

type Worker struct {
	logger   *slog.Logger
	store    pendingJobStore
	service  *Service
	interval time.Duration
}

func NewWorker(logger *slog.Logger, store pendingJobStore, service *Service, interval time.Duration) *Worker {
	return &Worker{
		logger:   logger,
		store:    store,
		service:  service,
		interval: interval,
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
	ids, err := w.store.ListPendingIDs(ctx, 10)
	if err != nil {
		return err
	}

	for _, id := range ids {
		job, err := w.service.ProcessPending(ctx, id)
		if err != nil {
			w.logger.Error("process pending sync job failed", "sync_job_id", id, "error", err)
			continue
		}
		w.logger.Info("processed pending sync job", "sync_job_id", id, "status", job.Status)
	}

	return nil
}
