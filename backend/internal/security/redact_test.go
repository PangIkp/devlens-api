package security

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	t.Parallel()

	input := "Authorization: Bearer abc123 token=xyz password=secret X-Hub-Signature-256: sha256=deadbeef -----BEGIN RSA PRIVATE KEY-----\nsecret\n-----END RSA PRIVATE KEY-----"
	output := RedactSecrets(input)

	for _, fragment := range []string{"abc123", "xyz", "deadbeef", "BEGIN RSA PRIVATE KEY"} {
		if strings.Contains(output, fragment) {
			t.Fatalf("expected output to redact %q: %s", fragment, output)
		}
	}
}
