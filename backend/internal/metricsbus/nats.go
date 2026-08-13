package metricsbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/observability"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	streamName      = "METRICS"
	eventSubject    = "repository.sync.completed"
	workSubject     = "metrics.calculate"
	dlqSubject      = "metrics.calculate.dlq"
	durableName     = "metrics-calculator"
	ackWait         = 30 * time.Second
	maxDeliver      = 5
	historyStart    = "1970-01-01"
	publisherName   = "devlens-metrics"
	traceNamePrefix = "devlens/nats"
)

type Client struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	metrics *observability.Metrics
}

type fallbackCalculator interface {
	CalculateRepositoryMetrics(context.Context, string, metrics.CalculationRequest) error
}

func calculationRequestForEvent(event syncjob.SyncCompletedEvent) metrics.CalculationRequest {
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

	return metrics.CalculationRequest{
		From:          from.UTC(),
		To:            to.UTC(),
		MetricVersion: metrics.CurrentMetricVersion,
	}
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

func (c *Client) Check(context.Context) error {
	if c == nil || c.conn == nil || !c.conn.IsConnected() {
		return fmt.Errorf("nats connection unavailable")
	}
	return nil
}

func (c *Client) PublishRepositorySyncCompleted(ctx context.Context, event syncjob.SyncCompletedEvent) error {
	if c == nil {
		return fmt.Errorf("metrics bus is not configured")
	}
	started := time.Now()
	ctx, span := otel.Tracer("devlens/nats").Start(ctx, "nats.publish.repository_sync_completed")
	defer span.End()

	payload, err := json.Marshal(event)
	if err != nil {
		if c.metrics != nil {
			c.metrics.RecordQueuePublish("metricsbus", workSubject, "error")
		}
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal metrics event: %w", err)
	}

	_, err = c.js.PublishMsg(&nats.Msg{
		Subject: eventSubject,
		Data:    payload,
		Header:  nats.Header{"Nats-Msg-Id": []string{event.SyncJobID + ":event"}},
	})
	if err != nil {
		if c.metrics != nil {
			c.metrics.RecordQueuePublish("metricsbus", eventSubject, "error")
		}
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publish sync completed event: %w", err)
	}

	_, err = c.js.PublishMsg(&nats.Msg{
		Subject: workSubject,
		Data:    payload,
		Header:  nats.Header{"Nats-Msg-Id": []string{event.SyncJobID + ":metrics"}},
	})
	if err != nil {
		if c.metrics != nil {
			c.metrics.RecordQueuePublish("metricsbus", workSubject, "error")
		}
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publish metrics event: %w", err)
	}
	if c.metrics != nil {
		c.metrics.RecordQueuePublish("metricsbus", eventSubject, "ok")
		c.metrics.RecordQueuePublish("metricsbus", workSubject, "ok")
	}
	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination.name", workSubject),
		attribute.String("devlens.sync_job_id", event.SyncJobID),
		attribute.Int64("devlens.nats.publish_ms", time.Since(started).Milliseconds()),
	)
	span.SetStatus(codes.Ok, "")

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
		if err := p.calculator.CalculateRepositoryMetrics(ctx, event.RepositoryID, calculationRequestForEvent(event)); err != nil {
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
	metrics    *observability.Metrics
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
		metrics:    client.metrics,
		calculator: calculator,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil {
		return nil
	}

	sub, err := c.js.Subscribe(workSubject, c.handleMessage, nats.Durable(durableName), nats.ManualAck(), nats.DeliverNew(), nats.AckWait(ackWait), nats.MaxDeliver(maxDeliver))
	if err != nil {
		return fmt.Errorf("subscribe metrics consumer: %w", err)
	}
	c.sub = sub
	defer c.sub.Unsubscribe()

	c.recordLag()

	<-ctx.Done()

	if err := c.sub.Drain(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
		return fmt.Errorf("drain metrics subscription: %w", err)
	}
	return nil
}

func (c *Consumer) handleMessage(msg *nats.Msg) {
	started := time.Now()
	ctx, span := otel.Tracer(traceNamePrefix).Start(context.Background(), "nats.consume.metrics_calculate")
	defer span.End()

	var event syncjob.SyncCompletedEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		c.logger.Error("decode metrics event failed", "error", err)
		if c.metrics != nil {
			c.metrics.RecordQueueConsume("metricsbus", workSubject, "decode_error", time.Since(started))
		}
		c.recordLag()
		span.SetStatus(codes.Error, err.Error())
		_ = msg.Ack()
		return
	}

	to := event.OccurredAt.UTC()
	if to.IsZero() {
		to = time.Now().UTC()
	}

	request := calculationRequestForEvent(event)
	request.To = to
	if err := c.calculator.CalculateRepositoryMetrics(ctx, event.RepositoryID, request); err != nil {
		c.logger.Error("calculate metrics failed", "repository_id", event.RepositoryID, "sync_job_id", event.SyncJobID, "error", err)
		if c.metrics != nil {
			c.metrics.RecordQueueConsume("metricsbus", workSubject, "error", time.Since(started))
		}
		c.recordLag()
		span.SetStatus(codes.Error, err.Error())
		if shouldDeadLetter(msg) {
			if dlqErr := c.publishDeadLetter(msg, err); dlqErr != nil {
				c.logger.Error("publish dead-letter failed", "error", dlqErr)
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
		c.metrics.RecordQueueConsume("metricsbus", workSubject, "ok", time.Since(started))
	}
	c.recordLag()
	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.destination.name", workSubject),
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
	c.metrics.RecordQueueLag("metricsbus", workSubject, int64(info.NumPending))
}

func (c *Client) ensureStream() error {
	info, err := c.js.StreamInfo(streamName)
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("inspect metrics stream: %w", err)
	}

	desired := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{eventSubject, workSubject, dlqSubject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	}

	if errors.Is(err, nats.ErrStreamNotFound) {
		if _, err := c.js.AddStream(desired); err != nil {
			return fmt.Errorf("create metrics stream: %w", err)
		}
		return nil
	}

	if metricsStreamMatches(info.Config, *desired) {
		return nil
	}

	updated := info.Config
	updated.Subjects = append([]string(nil), desired.Subjects...)
	updated.Retention = desired.Retention
	updated.Storage = desired.Storage

	if _, err := c.js.UpdateStream(&updated); err != nil {
		return fmt.Errorf("update metrics stream: %w", err)
	}

	return nil
}

func metricsStreamMatches(current nats.StreamConfig, desired nats.StreamConfig) bool {
	return current.Name == desired.Name &&
		current.Retention == desired.Retention &&
		current.Storage == desired.Storage &&
		slices.Equal(sortedSubjects(current.Subjects), sortedSubjects(desired.Subjects))
}

func sortedSubjects(subjects []string) []string {
	items := append([]string(nil), subjects...)
	slices.Sort(items)
	return items
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
		return fmt.Errorf("metrics dead-letter publisher is not configured")
	}
	headers := nats.Header{}
	for key, values := range msg.Header {
		copyValues := append([]string(nil), values...)
		headers[key] = copyValues
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
		return fmt.Errorf("publish dead-letter message: %w", err)
	}
	if c.metrics != nil {
		c.metrics.RecordQueuePublish("metricsbus", dlqSubject, "ok")
	}
	return nil
}
