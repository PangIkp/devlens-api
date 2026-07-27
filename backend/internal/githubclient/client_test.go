package githubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGetRepositoryIncludesAuthHeaders(t *testing.T) {
	t.Parallel()

	doer := &stubHTTPDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/repos/devlens-labs/devlens-api" {
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected authorization header %q", got)
			}
			if got := req.Header.Get("User-Agent"); got != "devlens-test" {
				t.Fatalf("unexpected user agent %q", got)
			}
			if got := req.Header.Get("Accept"); got != "application/vnd.github+json" {
				t.Fatalf("unexpected accept header %q", got)
			}

			return jsonResponse(http.StatusOK, Repository{
				ID:            42,
				Name:          "devlens-api",
				FullName:      "devlens-labs/devlens-api",
				DefaultBranch: "main",
			}), nil
		},
	}

	client := newTestClient(t, doer)

	repo, err := client.GetRepository(context.Background(), "devlens-labs", "devlens-api")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.ID != 42 || repo.FullName != "devlens-labs/devlens-api" {
		t.Fatalf("unexpected repository %+v", repo)
	}
}

func TestGetRepositoryWithoutTokenOmitsAuthorizationHeader(t *testing.T) {
	t.Parallel()

	doer := &stubHTTPDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("Authorization"); got != "" {
				t.Fatalf("expected empty authorization header, got %q", got)
			}
			return jsonResponse(http.StatusOK, Repository{ID: 1, FullName: "devlens-labs/devlens-api"}), nil
		},
	}

	client := newTestClientWithProvider(t, doer, StaticTokenProvider{})

	_, err := client.GetRepository(context.Background(), "devlens-labs", "devlens-api")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestListPullRequestsParsesPaginationAndRateLimit(t *testing.T) {
	t.Parallel()

	doer := &stubHTTPDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			if got := req.URL.Query().Get("page"); got != "2" {
				t.Fatalf("unexpected page %q", got)
			}
			if got := req.URL.Query().Get("per_page"); got != "50" {
				t.Fatalf("unexpected per_page %q", got)
			}
			if got := req.URL.Query().Get("state"); got != "all" {
				t.Fatalf("unexpected state %q", got)
			}

			resp := jsonResponse(http.StatusOK, []PullRequest{{
				ID:     1001,
				Number: 7,
				Title:  "Add github sync",
				State:  "open",
				User:   User{Login: "alice", ID: 99},
			}})
			resp.Header.Set("Link", `<https://api.github.com/resource?page=3>; rel="next", <https://api.github.com/resource?page=10>; rel="last"`)
			resp.Header.Set("X-RateLimit-Limit", "5000")
			resp.Header.Set("X-RateLimit-Remaining", "4998")
			resp.Header.Set("X-RateLimit-Reset", "1785153600")
			return resp, nil
		},
	}

	client := newTestClient(t, doer)

	page, err := client.ListPullRequests(context.Background(), "devlens-labs", "devlens-api", ListOptions{
		Page:    2,
		PerPage: 50,
		State:   "all",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if page.NextPage != 3 {
		t.Fatalf("expected next page 3, got %d", page.NextPage)
	}
	if page.RateLimit.Limit != 5000 || page.RateLimit.Remaining != 4998 {
		t.Fatalf("unexpected rate limit %+v", page.RateLimit)
	}
	if len(page.Items) != 1 || page.Items[0].Number != 7 {
		t.Fatalf("unexpected items %+v", page.Items)
	}
}

func TestGetPullRequestReturnsDetailFields(t *testing.T) {
	t.Parallel()

	doer := &stubHTTPDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/repos/devlens-labs/devlens-api/pulls/7" {
				t.Fatalf("unexpected path %q", req.URL.Path)
			}

			return jsonResponse(http.StatusOK, PullRequest{
				ID:           1001,
				Number:       7,
				Title:        "Add github sync",
				State:        "closed",
				User:         User{Login: "alice"},
				Additions:    120,
				Deletions:    15,
				ChangedFiles: 4,
			}), nil
		},
	}

	client := newTestClient(t, doer)

	item, err := client.GetPullRequest(context.Background(), "devlens-labs", "devlens-api", 7)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if item.Additions != 120 || item.Deletions != 15 || item.ChangedFiles != 4 {
		t.Fatalf("unexpected pull request %+v", item)
	}
}

func TestListReviewsAndCommitsUseDefaultPagination(t *testing.T) {
	t.Parallel()

	var seen []string
	doer := &stubHTTPDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			seen = append(seen, req.URL.Path+"?"+req.URL.RawQuery)
			switch req.URL.Path {
			case "/repos/devlens-labs/devlens-api/pulls/5/reviews":
				return jsonResponse(http.StatusOK, []Review{{ID: 1, State: "APPROVED", CommitID: "abc"}}), nil
			case "/repos/devlens-labs/devlens-api/commits":
				return jsonResponse(http.StatusOK, []Commit{{SHA: "deadbeef", Commit: CommitDetail{Message: "feat: sync"}}}), nil
			default:
				t.Fatalf("unexpected path %q", req.URL.Path)
				return nil, nil
			}
		},
	}

	client := newTestClient(t, doer)

	reviews, err := client.ListReviews(context.Background(), "devlens-labs", "devlens-api", 5, ListOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	commits, err := client.ListCommits(context.Background(), "devlens-labs", "devlens-api", ListOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{
		"/repos/devlens-labs/devlens-api/pulls/5/reviews?page=1&per_page=30",
		"/repos/devlens-labs/devlens-api/commits?page=1&per_page=30",
	}
	if !reflect.DeepEqual(seen, expected) {
		t.Fatalf("unexpected requests %v", seen)
	}
	if len(reviews.Items) != 1 || reviews.Items[0].State != "APPROVED" {
		t.Fatalf("unexpected reviews %+v", reviews.Items)
	}
	if len(commits.Items) != 1 || commits.Items[0].SHA != "deadbeef" {
		t.Fatalf("unexpected commits %+v", commits.Items)
	}
}

func TestClientRetriesTemporaryFailures(t *testing.T) {
	t.Parallel()

	attempts := 0
	doer := &stubHTTPDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			attempts++
			if !strings.Contains(req.URL.Path, "/repos/devlens-labs/devlens-api") {
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			if attempts < 3 {
				return jsonResponse(http.StatusBadGateway, map[string]string{"message": "temporary failure"}), nil
			}
			return jsonResponse(http.StatusOK, Repository{ID: 42, FullName: "devlens-labs/devlens-api"}), nil
		},
	}

	client := newTestClient(t, doer)
	var waits []time.Duration
	client.maxRetries = 3
	client.sleep = func(_ context.Context, waitFor time.Duration) error {
		waits = append(waits, waitFor)
		return nil
	}

	repo, err := client.GetRepository(context.Background(), "devlens-labs", "devlens-api")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.ID != 42 {
		t.Fatalf("unexpected repository %+v", repo)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if !reflect.DeepEqual(waits, []time.Duration{time.Millisecond, 2 * time.Millisecond}) {
		t.Fatalf("unexpected waits %v", waits)
	}
}

func TestClientRetriesSecondaryRateLimit(t *testing.T) {
	t.Parallel()

	attempts := 0
	doer := &stubHTTPDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				resp := jsonResponse(http.StatusForbidden, map[string]string{
					"message": "You have exceeded a secondary rate limit.",
				})
				resp.Header.Set("Retry-After", "1")
				return resp, nil
			}
			return jsonResponse(http.StatusOK, Repository{ID: 88, FullName: "devlens-labs/devlens-api"}), nil
		},
	}

	client := newTestClient(t, doer)
	var waits []time.Duration
	client.sleep = func(_ context.Context, waitFor time.Duration) error {
		waits = append(waits, waitFor)
		return nil
	}

	_, err := client.GetRepository(context.Background(), "devlens-labs", "devlens-api")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if !reflect.DeepEqual(waits, []time.Duration{time.Second}) {
		t.Fatalf("unexpected waits %v", waits)
	}
}

type stubHTTPDoer struct {
	doFn func(*http.Request) (*http.Response, error)
}

func (s *stubHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	return s.doFn(req)
}

func newTestClient(t *testing.T, doer HTTPDoer) *HTTPClient {
	t.Helper()
	return newTestClientWithProvider(t, doer, StaticTokenProvider{Value: "test-token"})
}

func newTestClientWithProvider(t *testing.T, doer HTTPDoer, provider TokenProvider) *HTTPClient {
	t.Helper()

	client, err := New(Config{
		BaseURL:        "https://api.github.test",
		UserAgent:      "devlens-test",
		HTTPTimeout:    time.Second,
		MaxRetries:     2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
	}, provider)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	client.httpClient = doer

	return client
}

func jsonResponse(status int, payload any) *http.Response {
	body, _ := json.Marshal(payload)

	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
