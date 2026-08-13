package githubwebhook

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type stubRetryProcessor struct {
	mu     sync.Mutex
	calls  []int
	callCh chan int
}

func (s *stubRetryProcessor) RetryFailedPending(_ context.Context, limit int) error {
	s.mu.Lock()
	s.calls = append(s.calls, limit)
	s.mu.Unlock()
	if s.callCh != nil {
		s.callCh <- limit
	}
	return nil
}

func TestWorkerRunsRetryLoopImmediatelyAndOnInterval(t *testing.T) {
	t.Parallel()

	processor := &stubRetryProcessor{callCh: make(chan int, 4)}
	worker := NewWorker(slog.New(slog.NewTextHandler(io.Discard, nil)), processor, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case limit := <-processor.callCh:
		if limit != 10 {
			t.Fatalf("unexpected retry limit %d", limit)
		}
	case <-time.After(time.Second):
		t.Fatal("expected immediate retry loop execution")
	}

	select {
	case <-processor.callCh:
	case <-time.After(time.Second):
		t.Fatal("expected interval-based retry execution")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected worker to stop after context cancellation")
	}
}
