package metricsbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/nats-io/nats.go"
)

func TestPublishRepositorySyncCompletedIntegration(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	client, err := Open(cfg.NATS.URL)
	if err != nil {
		t.Skipf("skip nats integration test: nats unavailable: %v", err)
	}
	t.Cleanup(client.Close)

	sub, err := client.js.SubscribeSync(subjectName, nats.DeliverNew())
	if err != nil {
		t.Fatalf("subscribe to metrics subject: %v", err)
	}
	t.Cleanup(func() {
		_ = sub.Unsubscribe()
	})

	event := syncjob.SyncCompletedEvent{
		RepositoryID: "repo-integration",
		SyncJobID:    "job-" + time.Now().UTC().Format("20060102150405.000000000"),
		OccurredAt:   time.Now().UTC(),
		EventType:    "repository.sync.completed",
	}

	if err := client.PublishRepositorySyncCompleted(context.Background(), event); err != nil {
		t.Fatalf("publish metrics event: %v", err)
	}

	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("read published metrics event: %v", err)
	}

	var received syncjob.SyncCompletedEvent
	if err := json.Unmarshal(msg.Data, &received); err != nil {
		t.Fatalf("decode published metrics event: %v", err)
	}

	if received.RepositoryID != event.RepositoryID {
		t.Fatalf("unexpected repository id %q", received.RepositoryID)
	}
	if received.SyncJobID != event.SyncJobID {
		t.Fatalf("unexpected sync job id %q", received.SyncJobID)
	}
}
