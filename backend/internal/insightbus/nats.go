package insightbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PangIkp/devlens/backend/internal/insights"
	"github.com/PangIkp/devlens/backend/internal/observability"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	streamName    = "INSIGHTS"
	workSubject   = "insights.generate"
	dlqSubject    = "insights.generate.dlq"
	durableName   = "insight-generator"
	ackWait       = 30 * time.Second
	maxDeliver    = 5
	historyStart  = "1970-01-01"
	publisherName = "devlens-insights"
)

type Client struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	metrics *observability.Metrics
}

type generator interface {
	RefreshRepository(context.Context, string, string, time.Time, time.Time) error
}

func Open(url string, metrics *observability.Metrics) (*Client, error) {
	conn, err := nats.Connect(url, nats.Name(publisherName))
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	client := &Client{conn: conn, js: js, metrics: metrics}
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
		return fmt.Errorf("insight bus is not configured")
	}
	started := time.Now()
	ctx, span := otel.Tracer("devlens/nats").Start(ctx, "nats.publish.insights_generate")
	defer span.End()

	payload, err := json.Marshal(event)
	if err != nil {
		if c.metrics != nil {
			c.metrics.RecordQueuePublish("insightbus", workSubject, "error")
		}
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal insight event: %w", err)
	}

	_, err = c.js.PublishMsg(&nats.Msg{
		Subject: workSubject,
		Data:    payload,
		Header:  nats.Header{"Nats-Msg-Id": []string{event.SyncJobID + ":insights"}},
	})
	if err != nil {
		if c.metrics != nil {
			c.metrics.RecordQueuePublish("insightbus", workSubject, "error")
		}
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publish insight event: %w", err)
	}
	if c.metrics != nil {
		c.metrics.RecordQueuePublish("insightbus", workSubject, "ok")
	}
	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination.name", workSubject),
		attribute.String("devlens.organization_id", event.OrganizationID),
		attribute.String("devlens.repository_id", event.RepositoryID),
		attribute.String("devlens.sync_job_id", event.SyncJobID),
		attribute.Int64("devlens.nats.publish_ms", time.Since(started).Milliseconds()),
	)
	span.SetStatus(codes.Ok, "")
	return nil
}

type Publisher struct {
	logger *slog.Logger
	client *Client
}

func NewPublisher(logger *slog.Logger, client *Client) *Publisher {
	if client == nil {
		return nil
	}
	return &Publisher{logger: logger, client: client}
}

func (p *Publisher) PublishRepositorySyncCompleted(ctx context.Context, event syncjob.SyncCompletedEvent) error {
	if p == nil || p.client == nil {
		return nil
	}
	if err := p.client.PublishRepositorySyncCompleted(ctx, event); err != nil {
		if p.logger != nil {
			p.logger.Warn("publish insight event failed", "sync_job_id", event.SyncJobID, "repository_id", event.RepositoryID, "organization_id", event.OrganizationID, "error", err)
		}
		return err
	}
	return nil
}

type Consumer struct {
	logger    *slog.Logger
	js        nats.JetStreamContext
	sub       *nats.Subscription
	metrics   *observability.Metrics
	generator generator
}

func NewConsumer(logger *slog.Logger, client *Client, generator generator) *Consumer {
	if client == nil {
		return nil
	}
	return &Consumer{
		logger:    logger,
		js:        client.js,
		metrics:   client.metrics,
		generator: generator,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil {
		return nil
	}

	sub, err := c.js.Subscribe(workSubject, c.handleMessage, nats.Durable(durableName), nats.ManualAck(), nats.DeliverNew(), nats.AckWait(ackWait), nats.MaxDeliver(maxDeliver))
	if err != nil {
		return fmt.Errorf("subscribe insight consumer: %w", err)
	}
	c.sub = sub
	defer c.sub.Unsubscribe()

	c.recordLag()
	<-ctx.Done()

	if err := c.sub.Drain(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
		return fmt.Errorf("drain insight subscription: %w", err)
	}
	return nil
}

func (c *Consumer) handleMessage(msg *nats.Msg) {
	started := time.Now()
	ctx, span := otel.Tracer("devlens/nats").Start(context.Background(), "nats.consume.insights_generate")
	defer span.End()

	var event syncjob.SyncCompletedEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		c.logger.Error("decode insight event failed", "error", err)
		if c.metrics != nil {
			c.metrics.RecordQueueConsume("insightbus", workSubject, "decode_error", time.Since(started))
		}
		c.recordLag()
		span.SetStatus(codes.Error, err.Error())
		_ = msg.Ack()
		return
	}

	from := event.From.UTC()
	if from.IsZero() {
		from, _ = time.Parse("2006-01-02", historyStart)
	}
	to := event.To.UTC()
	if to.IsZero() {
		to = event.OccurredAt.UTC()
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}

	if err := c.generator.RefreshRepository(ctx, event.OrganizationID, event.RepositoryID, from, to); err != nil {
		c.logger.Error("refresh insights failed", "organization_id", event.OrganizationID, "repository_id", event.RepositoryID, "sync_job_id", event.SyncJobID, "error", err)
		if c.metrics != nil {
			c.metrics.RecordQueueConsume("insightbus", workSubject, "error", time.Since(started))
		}
		c.recordLag()
		span.SetStatus(codes.Error, err.Error())
		if shouldDeadLetter(msg) {
			if dlqErr := c.publishDeadLetter(msg, err); dlqErr != nil {
				c.logger.Error("publish insight dead-letter failed", "error", dlqErr)
				_ = msg.Nak()
				return
			}
			_ = msg.Ack()
			return
		}
		_ = msg.Nak()
		return
	}

	if c.metrics != nil {
		c.metrics.RecordQueueConsume("insightbus", workSubject, "ok", time.Since(started))
	}
	c.recordLag()
	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination.name", workSubject),
		attribute.String("devlens.organization_id", event.OrganizationID),
		attribute.String("devlens.repository_id", event.RepositoryID),
		attribute.String("devlens.sync_job_id", event.SyncJobID),
		attribute.Int64("devlens.nats.consume_ms", time.Since(started).Milliseconds()),
	)
	span.SetStatus(codes.Ok, "")
	_ = msg.Ack()
}

func (c *Consumer) recordLag() {
	if c == nil || c.metrics == nil || c.js == nil {
		return
	}
	info, err := c.js.ConsumerInfo(streamName, durableName)
	if err != nil || info == nil {
		return
	}
	c.metrics.RecordQueueLag("insightbus", workSubject, int64(info.NumPending))
}

func (c *Client) ensureStream() error {
	_, err := c.js.StreamInfo(streamName)
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("inspect insight stream: %w", err)
	}
	_, err = c.js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{workSubject, dlqSubject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("create insight stream: %w", err)
	}
	return nil
}

func shouldDeadLetter(msg *nats.Msg) bool {
	if msg == nil {
		return false
	}
	meta, err := msg.Metadata()
	if err != nil || meta == nil {
		return false
	}
	return meta.NumDelivered >= maxDeliver
}

func (c *Consumer) publishDeadLetter(msg *nats.Msg, reason error) error {
	if c == nil || c.js == nil || msg == nil {
		return fmt.Errorf("insight dead-letter publisher is not configured")
	}
	headers := nats.Header{}
	for key, values := range msg.Header {
		headers[key] = append([]string(nil), values...)
	}
	headers.Set("X-DevLens-DLQ-Reason", reason.Error())
	if meta, err := msg.Metadata(); err == nil && meta != nil {
		headers.Set("X-DevLens-DLQ-Deliveries", fmt.Sprintf("%d", meta.NumDelivered))
	}
	_, err := c.js.PublishMsg(&nats.Msg{
		Subject: dlqSubject,
		Data:    msg.Data,
		Header:  headers,
	})
	if err != nil {
		return fmt.Errorf("publish insight dead-letter message: %w", err)
	}
	if c.metrics != nil {
		c.metrics.RecordQueuePublish("insightbus", dlqSubject, "ok")
	}
	return nil
}

var _ generator = (*insights.Service)(nil)
