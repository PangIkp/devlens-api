package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type config struct {
	scenario     string
	baseURL      string
	concurrency  int
	requests     int
	timeout      time.Duration
	repositoryID string
	from         string
	to           string
	page         int
	pageSize     int
	token        string
	webhookEvent string
	webhookBody  string
	webhookKey   string
}

type result struct {
	statusCode int
	duration   time.Duration
	err        error
}

func main() {
	cfg := loadConfig()
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(2)
	}

	client := &http.Client{Timeout: cfg.timeout}
	jobs := make(chan int)
	results := make(chan result, cfg.requests)

	var wg sync.WaitGroup
	for workerID := 0; workerID < cfg.concurrency; workerID++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := range jobs {
				results <- execute(client, cfg, seq)
			}
		}()
	}

	started := time.Now()
	for i := 0; i < cfg.requests; i++ {
		jobs <- i + 1
	}
	close(jobs)
	wg.Wait()
	close(results)

	var (
		successes uint64
		failures  uint64
		durations []time.Duration
	)

	for item := range results {
		if item.err != nil || item.statusCode < 200 || item.statusCode >= 300 {
			atomic.AddUint64(&failures, 1)
		} else {
			atomic.AddUint64(&successes, 1)
		}
		durations = append(durations, item.duration)
	}

	totalDuration := time.Since(started)
	printSummary(cfg, totalDuration, durations, successes, failures)
}

func loadConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.scenario, "scenario", "dashboard", "load test scenario: dashboard or webhook")
	flag.StringVar(&cfg.baseURL, "base-url", "http://localhost:8080/api/v1", "base API URL")
	flag.IntVar(&cfg.concurrency, "concurrency", 10, "number of concurrent workers")
	flag.IntVar(&cfg.requests, "requests", 100, "total number of requests")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "per-request timeout")
	flag.StringVar(&cfg.repositoryID, "repository-id", "", "repository ID for dashboard scenario")
	flag.StringVar(&cfg.from, "from", "2026-08-01", "from date for dashboard scenario (YYYY-MM-DD)")
	flag.StringVar(&cfg.to, "to", "2026-08-13", "to date for dashboard scenario (YYYY-MM-DD)")
	flag.IntVar(&cfg.page, "page", 1, "page for dashboard scenario")
	flag.IntVar(&cfg.pageSize, "page-size", 20, "page size for dashboard scenario")
	flag.StringVar(&cfg.webhookEvent, "webhook-event", "pull_request", "GitHub webhook event name")
	flag.StringVar(&cfg.webhookBody, "webhook-body", `{"action":"opened","repository":{"id":42,"full_name":"pangikp/devlens-api"}}`, "GitHub webhook JSON payload")
	flag.Parse()

	cfg.token = strings.TrimSpace(os.Getenv("LOADTEST_BEARER_TOKEN"))
	cfg.webhookKey = strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	return cfg
}

func validateConfig(cfg config) error {
	switch cfg.scenario {
	case "dashboard":
		if strings.TrimSpace(cfg.repositoryID) == "" {
			return fmt.Errorf("repository-id is required for dashboard scenario")
		}
	case "webhook":
		if cfg.webhookKey == "" {
			return fmt.Errorf("GITHUB_WEBHOOK_SECRET is required for webhook scenario")
		}
	default:
		return fmt.Errorf("unsupported scenario %q", cfg.scenario)
	}

	if cfg.concurrency < 1 {
		return fmt.Errorf("concurrency must be greater than 0")
	}
	if cfg.requests < 1 {
		return fmt.Errorf("requests must be greater than 0")
	}
	if cfg.timeout <= 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}

	return nil
}

func execute(client *http.Client, cfg config, seq int) result {
	started := time.Now()
	req, err := buildRequest(context.Background(), cfg, seq)
	if err != nil {
		return result{duration: time.Since(started), err: err}
	}

	resp, err := client.Do(req)
	if err != nil {
		return result{duration: time.Since(started), err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return result{
		statusCode: resp.StatusCode,
		duration:   time.Since(started),
	}
}

func buildRequest(ctx context.Context, cfg config, seq int) (*http.Request, error) {
	switch cfg.scenario {
	case "dashboard":
		url := fmt.Sprintf("%s/repositories/%s/dashboard/review-queue?from=%s&to=%s&page=%d&pageSize=%d",
			strings.TrimRight(cfg.baseURL, "/"),
			cfg.repositoryID,
			cfg.from,
			cfg.to,
			cfg.page,
			cfg.pageSize,
		)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if cfg.token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.token)
		}
		return req, nil
	case "webhook":
		body := []byte(cfg.webhookBody)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.baseURL, "/")+"/github/webhook", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", cfg.webhookEvent)
		req.Header.Set("X-GitHub-Delivery", fmt.Sprintf("loadtest-%d-%d", time.Now().UTC().UnixNano(), seq))
		req.Header.Set("X-Hub-Signature-256", sign(cfg.webhookKey, body))
		return req, nil
	default:
		return nil, fmt.Errorf("unsupported scenario %q", cfg.scenario)
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func printSummary(cfg config, total time.Duration, durations []time.Duration, successes uint64, failures uint64) {
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	totalRequests := successes + failures
	fmt.Printf("scenario:      %s\n", cfg.scenario)
	fmt.Printf("requests:      %d\n", totalRequests)
	fmt.Printf("concurrency:   %d\n", cfg.concurrency)
	fmt.Printf("successes:     %d\n", successes)
	fmt.Printf("failures:      %d\n", failures)
	fmt.Printf("total time:    %s\n", total.Truncate(time.Millisecond))
	if total > 0 {
		fmt.Printf("throughput:    %.2f req/s\n", float64(totalRequests)/total.Seconds())
	}
	if len(durations) == 0 {
		return
	}

	var sum time.Duration
	for _, item := range durations {
		sum += item
	}

	fmt.Printf("avg latency:   %s\n", (sum / time.Duration(len(durations))).Truncate(time.Millisecond))
	fmt.Printf("p50 latency:   %s\n", percentile(durations, 50).Truncate(time.Millisecond))
	fmt.Printf("p95 latency:   %s\n", percentile(durations, 95).Truncate(time.Millisecond))
	fmt.Printf("p99 latency:   %s\n", percentile(durations, 99).Truncate(time.Millisecond))
}

func percentile(items []time.Duration, p int) time.Duration {
	if len(items) == 0 {
		return 0
	}
	if p <= 0 {
		return items[0]
	}
	if p >= 100 {
		return items[len(items)-1]
	}
	index := (len(items)*p + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > len(items) {
		index = len(items)
	}
	return items[index-1]
}
