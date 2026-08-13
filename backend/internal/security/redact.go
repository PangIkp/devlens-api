package security

import (
	"regexp"
	"strings"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|token)\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(x-hub-signature-256:\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)(token=)[^&\s]+`),
	regexp.MustCompile(`(?i)(password=)[^&\s]+`),
}

func RedactSecrets(input string) string {
	redacted := input
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, redactMatch)
	}
	return redacted
}

func redactMatch(value string) string {
	for _, prefix := range []string{
		"authorization:",
		"x-hub-signature-256:",
		"token=",
		"password=",
	} {
		if strings.HasPrefix(strings.ToLower(value), prefix) {
			return value[:len(prefix)] + "[REDACTED]"
		}
	}
	return "[REDACTED]"
}
