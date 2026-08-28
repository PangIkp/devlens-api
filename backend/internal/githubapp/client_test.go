package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/config"
)

func TestCreateInstallationTokenClassifiesBadCredentials(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials","documentation_url":"https://docs.github.com/rest"}`))
	}))

	_, err := client.CreateInstallationToken(context.Background(), 123)
	if !errors.Is(err, ErrAppCredentialsInvalid) {
		t.Fatalf("expected ErrAppCredentialsInvalid, got %v", err)
	}
	if strings.Contains(err.Error(), "documentation_url") {
		t.Fatalf("expected sanitized error, got %q", err.Error())
	}
}

func TestCreateInstallationTokenClassifiesMissingInstallation(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations/123/access_tokens" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))

	_, err := client.CreateInstallationToken(context.Background(), 123)
	if !errors.Is(err, ErrInstallationNotFound) {
		t.Fatalf("expected ErrInstallationNotFound, got %v", err)
	}
}

func TestParsePrivateKeyAcceptsEscapedNewlines(t *testing.T) {
	t.Parallel()

	key := testPrivateKeyPEM(t)
	escaped := strings.ReplaceAll(key, "\n", `\n`)
	if _, err := parsePrivateKey(escaped); err != nil {
		t.Fatalf("parse escaped private key: %v", err)
	}
}

func newTestClient(t *testing.T, handler http.Handler) *HTTPClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := New(config.GitHubConfig{
		BaseURL:     server.URL,
		UserAgent:   "devlens-test",
		HTTPTimeout: time.Second,
		App: config.GitHubAppConfig{
			AppID:      12345,
			InstallURL: "https://github.com/apps/devlens/installations/new",
			PrivateKey: testPrivateKeyPEM(t),
		},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}
