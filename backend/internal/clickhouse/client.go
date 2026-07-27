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
)

type DB struct {
	baseURL  *url.URL
	database string
	client   *http.Client
}

func Open(cfg config.ClickHouseConfig) (*DB, error) {
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
	}, nil
}

func (db *DB) Check(ctx context.Context) error {
	return db.Exec(ctx, "SELECT 1")
}

func (db *DB) Close() {}

func (db *DB) Exec(ctx context.Context, query string) error {
	req, err := db.newRequest(ctx, query)
	if err != nil {
		return err
	}

	resp, err := db.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute clickhouse query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clickhouse query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return fmt.Errorf("drain clickhouse response: %w", err)
	}
	return nil
}

func QueryJSONEachRow[T any](ctx context.Context, db *DB, query string) ([]T, error) {
	req, err := db.newRequest(ctx, query+" FORMAT JSONEachRow")
	if err != nil {
		return nil, err
	}

	resp, err := db.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute clickhouse query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clickhouse query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	decoder := json.NewDecoder(resp.Body)
	results := make([]T, 0)
	for decoder.More() {
		var item T
		if err := decoder.Decode(&item); err != nil {
			return nil, fmt.Errorf("decode clickhouse json each row: %w", err)
		}
		results = append(results, item)
	}

	return results, nil
}

func (db *DB) InsertJSONEachRow(ctx context.Context, insertSQL string, rows any) error {
	value := reflect.ValueOf(rows)
	if value.Kind() != reflect.Slice {
		return fmt.Errorf("clickhouse insert rows must be a slice")
	}
	if value.Len() == 0 {
		return nil
	}

	body, err := marshalJSONEachRow(insertSQL, rows)
	if err != nil {
		return err
	}

	req, err := db.newRequest(ctx, body)
	if err != nil {
		return err
	}

	resp, err := db.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute clickhouse insert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clickhouse insert failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return fmt.Errorf("drain clickhouse insert response: %w", err)
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
