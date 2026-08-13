package contracttest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenAPIIncludesImplementedCoreRoutes(t *testing.T) {
	t.Parallel()

	content := mustReadOpenAPI(t)
	expectedRoutes := []string{
		"/auth/login:",
		"/auth/refresh:",
		"/auth/logout:",
		"/me:",
		"/organizations:",
		"/organizations/{organizationId}:",
		"/organizations/{organizationId}/members:",
		"/organizations/{organizationId}/github/connection:",
		"/pull-requests/{pullRequestId}:",
		"/repositories/{repositoryId}/dashboard/review-queue:",
		"/repositories/{repositoryId}/metrics:",
		"/repositories/{repositoryId}/metrics/hotspots:",
		"/organizations/{organizationId}/insights:",
		"/insights:",
		"/github/webhook:",
	}

	for _, route := range expectedRoutes {
		if !strings.Contains(content, route) {
			t.Fatalf("expected openapi to include route %q", route)
		}
	}
}

func TestOpenAPIDocumentsOperationalBehavior(t *testing.T) {
	t.Parallel()

	content := mustReadOpenAPI(t)
	expectedEntries := []string{
		"TooManyRequests:",
		"X-Trace-Id:",
		"TraceID:",
		"Responses include `X-Trace-Id` for request tracing.",
		"The API may return `429 Too Many Requests` when server-side rate limits are exceeded.",
	}

	for _, entry := range expectedEntries {
		if !strings.Contains(content, entry) {
			t.Fatalf("expected openapi to include %q", entry)
		}
	}
}

func mustReadOpenAPI(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file path")
	}

	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "docs", "openapi.yaml")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}

	return string(bytes)
}
