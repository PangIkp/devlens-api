package metricsbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/nats-io/nats.go"
)

const (
	streamName   = "METRICS"
	subjectName  = "repository.sync.completed"
	durableName  = "metrics-calculator"
	ackWait      = 30 * time.Second
	maxDeliver   = 5
	historyStart = "1970-01-01"
)

type Client struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

type fallbackCalculator interface {
	CalculateRepositoryMetrics(context.Context, string, metrics.CalculationRequest) error
}

func Open(url string) (*Client, error) {
	conn, err := nats.Connect(url, nats.Name("devlens-metrics"))
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	client := &Client{conn: conn, js: js}
	if err := client.ensureStream(); err != nil {
		conn.Close()
		return nil, err
	}

	return client, nil
}

func (c *Client) Close() {
	if c == nil || c.conn == nil {
		return
	}
	c.conn.Drain()
	c.conn.Close()
}

func (c *Client) PublishRepositorySyncCompleted(ctx context.Context, event syncjob.SyncCompletedEvent) error {
	if c == nil {
		return fmt.Errorf("metrics bus is not configured")
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal metrics event: %w", err)
	}

	_, err = c.js.PublishMsg(&nats.Msg{
		Subject: subjectName,
		Data:    payload,
		Header:  nats.Header{"Nats-Msg-Id": []string{event.SyncJobID}},
	})
	if err != nil {
		return fmt.Errorf("publish metrics event: %w", err)
	}

	return nil
}

type Publisher struct {
	logger     *slog.Logger
	client     *Client
	calculator fallbackCalculator
}

func NewPublisher(logger *slog.Logger, client *Client, calculator fallbackCalculator) *Publisher {
	return &Publisher{
		logger:     logger,
		client:     client,
		calculator: calculator,
	}
}

func (p *Publisher) PublishRepositorySyncCompleted(ctx context.Context, event syncjob.SyncCompletedEvent) error {
	var publishErr error

	if p.client != nil {
		publishErr = p.client.PublishRepositorySyncCompleted(ctx, event)
		if publishErr == nil {
			return nil
		}
		if p.logger != nil {
			p.logger.Warn("publish metrics event failed, falling back to inline calculation", "sync_job_id", event.SyncJobID, "repository_id", event.RepositoryID, "error", publishErr)
		}
	}

	if p.calculator != nil {
		from, _ := time.Parse("2006-01-02", historyStart)
		if err := p.calculator.CalculateRepositoryMetrics(ctx, event.RepositoryID, metrics.CalculationRequest{
			From: from.UTC(),
			To:   event.OccurredAt.UTC(),
		}); err != nil {
			if publishErr != nil {
				return fmt.Errorf("publish metrics event: %w; fallback calculation: %w", publishErr, err)
			}
			return fmt.Errorf("fallback metrics calculation: %w", err)
		}
		return nil
	}

	if publishErr != nil {
		return publishErr
	}

	return fmt.Errorf("metrics publisher is not configured")
}

type Consumer struct {
	logger     *slog.Logger
	js         nats.JetStreamContext
	sub        *nats.Subscription
	calculator interface {
		CalculateRepositoryMetrics(context.Context, string, metrics.CalculationRequest) error
	}
}

func NewConsumer(logger *slog.Logger, client *Client, calculator interface {
	CalculateRepositoryMetrics(context.Context, string, metrics.CalculationRequest) error
}) *Consumer {
	if client == nil {
		return nil
	}

	return &Consumer{
		logger:     logger,
		js:         client.js,
		calculator: calculator,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil {
		return nil
	}

	sub, err := c.js.Subscribe(subjectName, c.handleMessage, nats.Durable(durableName), nats.ManualAck(), nats.DeliverNew(), nats.AckWait(ackWait), nats.MaxDeliver(maxDeliver))
	if err != nil {
		return fmt.Errorf("subscribe metrics consumer: %w", err)
	}
	c.sub = sub
	defer c.sub.Unsubscribe()

	<-ctx.Done()

	if err := c.sub.Drain(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
		return fmt.Errorf("drain metrics subscription: %w", err)
	}
	return nil
}

func (c *Consumer) handleMessage(msg *nats.Msg) {
	var event syncjob.SyncCompletedEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		c.logger.Error("decode metrics event failed", "error", err)
		_ = msg.Ack()
		return
	}

	to := event.OccurredAt.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}

	from, _ := time.Parse("2006-01-02", historyStart)
	if err := c.calculator.CalculateRepositoryMetrics(context.Background(), event.RepositoryID, metrics.CalculationRequest{
		From: from.UTC(),
		To:   to,
	}); err != nil {
		c.logger.Error("calculate metrics failed", "repository_id", event.RepositoryID, "sync_job_id", event.SyncJobID, "error", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
}

func (c *Client) ensureStream() error {
	_, err := c.js.StreamInfo(streamName)
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("inspect metrics stream: %w", err)
	}

	_, err = c.js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subjectName},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create metrics stream: %w", err)
	}

	return nil
}
