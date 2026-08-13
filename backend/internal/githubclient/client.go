package githubclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const defaultPerPage = 30

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Client interface {
	GetRepository(context.Context, string, string) (Repository, error)
	GetPullRequest(context.Context, string, string, int) (PullRequest, error)
	ListPullRequests(context.Context, string, string, ListOptions) (Page[PullRequest], error)
	ListReviews(context.Context, string, string, int, ListOptions) (Page[Review], error)
	ListCommits(context.Context, string, string, ListOptions) (Page[Commit], error)
	ListPullRequestFiles(context.Context, string, string, int, ListOptions) (Page[PullRequestFile], error)
	ListWorkflowRuns(context.Context, string, string, ListOptions) (Page[WorkflowRun], error)
	ListDeployments(context.Context, string, string, ListOptions) (Page[Deployment], error)
	ListDeploymentStatuses(context.Context, string, string, int64, ListOptions) (Page[DeploymentStatus], error)
}

type TokenProvider interface {
	Token(context.Context) (string, error)
}

type Config struct {
	BaseURL        string
	UserAgent      string
	HTTPTimeout    time.Duration
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Metrics        *observability.Metrics
}

type HTTPClient struct {
	baseURL        *url.URL
	tokenProvider  TokenProvider
	userAgent      string
	httpClient     HTTPDoer
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	metrics        *observability.Metrics
	sleep          func(context.Context, time.Duration) error
}

type APIError struct {
	StatusCode int
	Message    string
}

type errorResponse struct {
	Message string `json:"message"`
}

type StaticTokenProvider struct {
	Value string
}

func (p StaticTokenProvider) Token(context.Context) (string, error) {
	return strings.TrimSpace(p.Value), nil
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api returned status %d: %s", e.StatusCode, e.Message)
}

func New(cfg Config, tokenProvider TokenProvider) (*HTTPClient, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse github base url: %w", err)
	}
	if tokenProvider == nil {
		return nil, errors.New("github token provider must not be nil")
	}
	if cfg.UserAgent == "" {
		return nil, errors.New("github user agent must not be empty")
	}
	if cfg.HTTPTimeout <= 0 {
		return nil, errors.New("github http timeout must be greater than 0")
	}
	if cfg.InitialBackoff <= 0 {
		return nil, errors.New("github initial backoff must be greater than 0")
	}
	if cfg.MaxBackoff <= 0 {
		return nil, errors.New("github max backoff must be greater than 0")
	}
	if cfg.InitialBackoff > cfg.MaxBackoff {
		return nil, errors.New("github initial backoff must be less than or equal to max backoff")
	}
	if cfg.MaxRetries < 0 {
		return nil, errors.New("github max retries must be greater than or equal to 0")
	}

	return &HTTPClient{
		baseURL:        baseURL,
		tokenProvider:  tokenProvider,
		userAgent:      cfg.UserAgent,
		httpClient:     &http.Client{Timeout: cfg.HTTPTimeout},
		maxRetries:     cfg.MaxRetries,
		initialBackoff: cfg.InitialBackoff,
		maxBackoff:     cfg.MaxBackoff,
		metrics:        cfg.Metrics,
		sleep:          sleepWithContext,
	}, nil
}

func (c *HTTPClient) GetRepository(ctx context.Context, owner, repo string) (Repository, error) {
	var result Repository
	_, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s", owner, repo), nil, &result)
	if err != nil {
		return Repository{}, err
	}
	return result, nil
}

func (c *HTTPClient) GetPullRequest(ctx context.Context, owner, repo string, pullNumber int) (PullRequest, error) {
	var result PullRequest
	_, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, pullNumber), nil, &result)
	if err != nil {
		return PullRequest{}, err
	}
	return result, nil
}

func (c *HTTPClient) ListPullRequests(ctx context.Context, owner, repo string, options ListOptions) (Page[PullRequest], error) {
	query := options.queryValues()
	var items []PullRequest
	meta, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), query, &items)
	if err != nil {
		return Page[PullRequest]{}, err
	}
	return Page[PullRequest]{Items: items, NextPage: meta.nextPage, RateLimit: meta.rateLimit}, nil
}

func (c *HTTPClient) ListReviews(ctx context.Context, owner, repo string, pullNumber int, options ListOptions) (Page[Review], error) {
	query := options.queryValues()
	var items []Review
	meta, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, pullNumber), query, &items)
	if err != nil {
		return Page[Review]{}, err
	}
	return Page[Review]{Items: items, NextPage: meta.nextPage, RateLimit: meta.rateLimit}, nil
}

func (c *HTTPClient) ListCommits(ctx context.Context, owner, repo string, options ListOptions) (Page[Commit], error) {
	query := options.queryValues()
	var items []Commit
	meta, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/commits", owner, repo), query, &items)
	if err != nil {
		return Page[Commit]{}, err
	}
	return Page[Commit]{Items: items, NextPage: meta.nextPage, RateLimit: meta.rateLimit}, nil
}

func (c *HTTPClient) ListPullRequestFiles(ctx context.Context, owner, repo string, pullNumber int, options ListOptions) (Page[PullRequestFile], error) {
	query := options.queryValues()
	var items []PullRequestFile
	meta, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, pullNumber), query, &items)
	if err != nil {
		return Page[PullRequestFile]{}, err
	}
	return Page[PullRequestFile]{Items: items, NextPage: meta.nextPage, RateLimit: meta.rateLimit}, nil
}

func (c *HTTPClient) ListWorkflowRuns(ctx context.Context, owner, repo string, options ListOptions) (Page[WorkflowRun], error) {
	query := options.queryValues()
	var payload WorkflowRunList
	meta, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/actions/runs", owner, repo), query, &payload)
	if err != nil {
		return Page[WorkflowRun]{}, err
	}
	return Page[WorkflowRun]{Items: payload.WorkflowRuns, NextPage: meta.nextPage, RateLimit: meta.rateLimit}, nil
}

func (c *HTTPClient) ListDeployments(ctx context.Context, owner, repo string, options ListOptions) (Page[Deployment], error) {
	query := options.queryValues()
	var items []Deployment
	meta, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/deployments", owner, repo), query, &items)
	if err != nil {
		return Page[Deployment]{}, err
	}
	return Page[Deployment]{Items: items, NextPage: meta.nextPage, RateLimit: meta.rateLimit}, nil
}

func (c *HTTPClient) ListDeploymentStatuses(ctx context.Context, owner, repo string, deploymentID int64, options ListOptions) (Page[DeploymentStatus], error) {
	query := options.queryValues()
	var items []DeploymentStatus
	meta, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/deployments/%d/statuses", owner, repo, deploymentID), query, &items)
	if err != nil {
		return Page[DeploymentStatus]{}, err
	}
	return Page[DeploymentStatus]{Items: items, NextPage: meta.nextPage, RateLimit: meta.rateLimit}, nil
}

type responseMeta struct {
	nextPage  int
	rateLimit RateLimit
}

func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, target any) (responseMeta, error) {
	var meta responseMeta

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		started := time.Now()
		spanCtx, span := otel.Tracer("devlens/github").Start(ctx, fmt.Sprintf("github %s %s", method, path))
		req, err := c.newRequest(ctx, method, path, query)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return responseMeta{}, err
		}
		span.SetAttributes(
			attribute.String("http.method", method),
			attribute.String("url.path", path),
			attribute.Int("devlens.github.attempt", attempt+1),
		)

		resp, err := c.httpClient.Do(req.WithContext(spanCtx))
		if err != nil {
			if c.metrics != nil {
				c.metrics.RecordGitHubRequest(method, path, 0, "transport_error", time.Since(started), -1)
			}
			span.SetStatus(codes.Error, err.Error())
			span.End()
			if attempt == c.maxRetries || !shouldRetryTransport(err) {
				return responseMeta{}, fmt.Errorf("perform github request: %w", err)
			}
			if err := c.sleep(ctx, c.backoffDuration(attempt)); err != nil {
				return responseMeta{}, err
			}
			continue
		}

		meta = responseMeta{
			nextPage:  parseNextPage(resp.Header.Get("Link")),
			rateLimit: parseRateLimit(resp.Header),
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			err = decodeJSONBody(resp.Body, target)
			if err != nil {
				if c.metrics != nil {
					c.metrics.RecordGitHubRequest(method, path, resp.StatusCode, "decode_error", time.Since(started), meta.rateLimit.Remaining)
				}
				span.SetStatus(codes.Error, err.Error())
				span.End()
				return responseMeta{}, err
			}
			if c.metrics != nil {
				c.metrics.RecordGitHubRequest(method, path, resp.StatusCode, "ok", time.Since(started), meta.rateLimit.Remaining)
			}
			span.SetAttributes(
				attribute.Int("http.status_code", resp.StatusCode),
				attribute.Int("devlens.github.rate_limit_remaining", meta.rateLimit.Remaining),
			)
			span.SetStatus(codes.Ok, "")
			span.End()
			return meta, nil
		}

		apiErr := parseAPIError(resp)
		if shouldRetryResponse(resp, apiErr) && attempt < c.maxRetries {
			if c.metrics != nil {
				c.metrics.RecordGitHubRequest(method, path, resp.StatusCode, "retry", time.Since(started), meta.rateLimit.Remaining)
			}
			span.SetAttributes(
				attribute.Int("http.status_code", resp.StatusCode),
				attribute.Int("devlens.github.rate_limit_remaining", meta.rateLimit.Remaining),
			)
			span.SetStatus(codes.Error, apiErr.Error())
			span.End()
			waitFor := retryDelay(resp, apiErr, c.backoffDuration(attempt))
			if err := c.sleep(ctx, waitFor); err != nil {
				return responseMeta{}, err
			}
			continue
		}

		if c.metrics != nil {
			c.metrics.RecordGitHubRequest(method, path, resp.StatusCode, "error", time.Since(started), meta.rateLimit.Remaining)
		}
		span.SetAttributes(
			attribute.Int("http.status_code", resp.StatusCode),
			attribute.Int("devlens.github.rate_limit_remaining", meta.rateLimit.Remaining),
		)
		span.SetStatus(codes.Error, apiErr.Error())
		span.End()
		return responseMeta{}, apiErr
	}

	return responseMeta{}, errors.New("github request retry loop exhausted")
}

func (c *HTTPClient) newRequest(ctx context.Context, method, path string, query url.Values) (*http.Request, error) {
	relative := &url.URL{Path: path, RawQuery: query.Encode()}
	endpoint := c.baseURL.ResolveReference(relative)

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.userAgent)
	token, err := c.tokenProvider.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("load github token: %w", err)
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}

	return req, nil
}

func (o ListOptions) queryValues() url.Values {
	values := url.Values{}
	page := o.Page
	if page <= 0 {
		page = 1
	}

	perPage := o.PerPage
	if perPage <= 0 {
		perPage = defaultPerPage
	}

	values.Set("page", strconv.Itoa(page))
	values.Set("per_page", strconv.Itoa(perPage))
	if state := strings.TrimSpace(o.State); state != "" {
		values.Set("state", state)
	}

	return values
}

func decodeJSONBody(body io.ReadCloser, target any) error {
	defer body.Close()

	if target == nil {
		_, err := io.Copy(io.Discard, body)
		if err != nil {
			return fmt.Errorf("discard github response body: %w", err)
		}
		return nil
	}

	if err := json.NewDecoder(body).Decode(target); err != nil {
		return fmt.Errorf("decode github response body: %w", err)
	}

	return nil
}

func parseAPIError(resp *http.Response) error {
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read github error response: %w", err)
	}

	if len(payload) == 0 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    http.StatusText(resp.StatusCode),
		}
	}

	var githubErr errorResponse
	if err := json.Unmarshal(payload, &githubErr); err == nil && strings.TrimSpace(githubErr.Message) != "" {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    githubErr.Message,
		}
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(payload)),
	}
}

func shouldRetryTransport(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}

func shouldRetryResponse(resp *http.Response, err error) bool {
	if resp == nil {
		return false
	}

	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusForbidden:
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			message := strings.ToLower(apiErr.Message)
			if strings.Contains(message, "secondary rate limit") || strings.Contains(message, "abuse detection") {
				return true
			}
		}
		if strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")) == "0" {
			return true
		}
	}

	return false
}

func retryDelay(resp *http.Response, err error, fallback time.Duration) time.Duration {
	if resp == nil {
		return fallback
	}

	if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
		if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}

	if resetAt := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); resetAt != "" {
		if epoch, parseErr := strconv.ParseInt(resetAt, 10, 64); parseErr == nil {
			waitFor := time.Until(time.Unix(epoch, 0))
			if waitFor > 0 {
				return waitFor
			}
		}
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		message := strings.ToLower(apiErr.Message)
		if strings.Contains(message, "secondary rate limit") && fallback < 2*time.Second {
			return 2 * time.Second
		}
	}

	return fallback
}

func parseNextPage(linkHeader string) int {
	if strings.TrimSpace(linkHeader) == "" {
		return 0
	}

	parts := strings.Split(linkHeader, ",")
	for _, part := range parts {
		section := strings.TrimSpace(part)
		if !strings.Contains(section, `rel="next"`) {
			continue
		}

		start := strings.Index(section, "<")
		end := strings.Index(section, ">")
		if start == -1 || end == -1 || end <= start+1 {
			return 0
		}

		nextURL, err := url.Parse(section[start+1 : end])
		if err != nil {
			return 0
		}

		page, err := strconv.Atoi(nextURL.Query().Get("page"))
		if err != nil {
			return 0
		}
		return page
	}

	return 0
}

func parseRateLimit(headers http.Header) RateLimit {
	result := RateLimit{}

	if limit, err := strconv.Atoi(strings.TrimSpace(headers.Get("X-RateLimit-Limit"))); err == nil {
		result.Limit = limit
	}
	if remaining, err := strconv.Atoi(strings.TrimSpace(headers.Get("X-RateLimit-Remaining"))); err == nil {
		result.Remaining = remaining
	}
	if reset, err := strconv.ParseInt(strings.TrimSpace(headers.Get("X-RateLimit-Reset")), 10, 64); err == nil && reset > 0 {
		result.ResetAt = time.Unix(reset, 0).UTC()
	}

	return result
}

func (c *HTTPClient) backoffDuration(attempt int) time.Duration {
	multiplier := math.Pow(2, float64(attempt))
	waitFor := time.Duration(float64(c.initialBackoff) * multiplier)
	if waitFor > c.maxBackoff {
		return c.maxBackoff
	}
	return waitFor
}

func sleepWithContext(ctx context.Context, waitFor time.Duration) error {
	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
