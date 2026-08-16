package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type DB struct {
	baseURL  *url.URL
	database string
	client   *http.Client
	metrics  *observability.Metrics
}

func Open(cfg config.ClickHouseConfig, metrics *observability.Metrics) (*DB, error) {
	baseURL, err := url.Parse(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse clickhouse dsn: %w", err)
	}

	if baseURL.User == nil && strings.TrimSpace(cfg.User) != "" {
		if strings.TrimSpace(cfg.Password) != "" {
			baseURL.User = url.UserPassword(strings.TrimSpace(cfg.User), cfg.Password)
		} else {
			baseURL.User = url.User(strings.TrimSpace(cfg.User))
		}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	httpClient := &http.Client{Timeout: timeout}
	return &DB{
		baseURL:  baseURL,
		database: strings.TrimSpace(cfg.Database),
		client:   httpClient,
		metrics:  metrics,
	}, nil
}

func (db *DB) Check(ctx context.Context) error {
	return db.Exec(ctx, "SELECT 1")
}

func (db *DB) Close() {}

func (db *DB) Exec(ctx context.Context, query string) error {
	started := time.Now()
	ctx, span := otel.Tracer("devlens/clickhouse").Start(ctx, "clickhouse.exec")
	defer span.End()
	req, err := db.newRequest(ctx, query)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	resp, err := db.client.Do(req)
	if err != nil {
		db.record("exec", "error", time.Since(started))
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("execute clickhouse query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		db.record("exec", "error", time.Since(started))
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		span.SetStatus(codes.Error, strings.TrimSpace(string(body)))
		return fmt.Errorf("clickhouse query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		db.record("exec", "error", time.Since(started))
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("drain clickhouse response: %w", err)
	}
	db.record("exec", "ok", time.Since(started))
	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	span.SetStatus(codes.Ok, "")
	return nil
}

func QueryJSONEachRow[T any](ctx context.Context, db *DB, query string) ([]T, error) {
	started := time.Now()
	ctx, span := otel.Tracer("devlens/clickhouse").Start(ctx, "clickhouse.query_json_each_row")
	defer span.End()
	req, err := db.newRequest(ctx, query+" FORMAT JSONEachRow")
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	resp, err := db.client.Do(req)
	if err != nil {
		db.record("query", "error", time.Since(started))
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("execute clickhouse query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		db.record("query", "error", time.Since(started))
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		span.SetStatus(codes.Error, strings.TrimSpace(string(body)))
		return nil, fmt.Errorf("clickhouse query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	decoder := json.NewDecoder(resp.Body)
	results := make([]T, 0)
	for decoder.More() {
		var item T
		if err := decoder.Decode(&item); err != nil {
			db.record("query", "error", time.Since(started))
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("decode clickhouse json each row: %w", err)
		}
		results = append(results, item)
	}

	db.record("query", "ok", time.Since(started))
	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	span.SetStatus(codes.Ok, "")
	return results, nil
}

func (db *DB) InsertJSONEachRow(ctx context.Context, insertSQL string, rows any) error {
	started := time.Now()
	ctx, span := otel.Tracer("devlens/clickhouse").Start(ctx, "clickhouse.insert_json_each_row")
	defer span.End()
	value := reflect.ValueOf(rows)
	if value.Kind() != reflect.Slice {
		span.SetStatus(codes.Error, "clickhouse insert rows must be a slice")
		return fmt.Errorf("clickhouse insert rows must be a slice")
	}
	if value.Len() == 0 {
		span.SetStatus(codes.Ok, "")
		return nil
	}

	body, err := marshalJSONEachRow(insertSQL, rows)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	req, err := db.newRequest(ctx, body)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	resp, err := db.client.Do(req)
	if err != nil {
		db.record("insert", "error", time.Since(started))
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("execute clickhouse insert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		db.record("insert", "error", time.Since(started))
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		span.SetStatus(codes.Error, strings.TrimSpace(string(payload)))
		return fmt.Errorf("clickhouse insert failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		db.record("insert", "error", time.Since(started))
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("drain clickhouse insert response: %w", err)
	}

	db.record("insert", "ok", time.Since(started))
	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	span.SetStatus(codes.Ok, "")
	return nil
}

func (db *DB) InsertJSONEachRowBatched(ctx context.Context, insertSQL string, rows any, batchSize int) error {
	value := reflect.ValueOf(rows)
	if value.Kind() != reflect.Slice {
		return fmt.Errorf("clickhouse insert rows must be a slice")
	}
	if value.Len() == 0 {
		return nil
	}
	if batchSize <= 0 || value.Len() <= batchSize {
		return db.InsertJSONEachRow(ctx, insertSQL, rows)
	}

	for start := 0; start < value.Len(); start += batchSize {
		end := start + batchSize
		if end > value.Len() {
			end = value.Len()
		}
		if err := db.InsertJSONEachRow(ctx, insertSQL, value.Slice(start, end).Interface()); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) newRequest(ctx context.Context, query string) (*http.Request, error) {
	endpoint := *db.baseURL
	queryParams := endpoint.Query()
	if db.database != "" {
		queryParams.Set("database", db.database)
	}
	endpoint.RawQuery = queryParams.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("create clickhouse request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	return req, nil
}

func marshalJSONEachRow(insertSQL string, rows any) (string, error) {
	value := reflect.ValueOf(rows)
	if value.Kind() != reflect.Slice {
		return "", fmt.Errorf("clickhouse insert rows must be a slice")
	}

	var buffer bytes.Buffer
	buffer.WriteString(insertSQL)
	buffer.WriteString(" FORMAT JSONEachRow\n")

	for i := 0; i < value.Len(); i++ {
		encoded, err := json.Marshal(value.Index(i).Interface())
		if err != nil {
			return "", fmt.Errorf("encode clickhouse row: %w", err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}

	return buffer.String(), nil
}

func (db *DB) record(operation, result string, duration time.Duration) {
	if db == nil || db.metrics == nil {
		return
	}
	db.metrics.RecordClickHouseQuery(operation, result, duration)
}
