package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PangIkp/devlens/backend/internal/auth"
)

type RateLimiter struct {
	mu       sync.Mutex
	entries  map[string]rateLimitEntry
	now      func() time.Time
	requests int
	window   time.Duration
}

type rateLimitEntry struct {
	count    int
	resetAt  time.Time
	lastSeen time.Time
}

func NewRateLimiter(requests int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries:  make(map[string]rateLimitEntry),
		now:      time.Now,
		requests: requests,
		window:   window,
	}
}

func (l *RateLimiter) Allow(key string) (bool, time.Time) {
	if l == nil || l.requests <= 0 || l.window <= 0 {
		return true, time.Time{}
	}

	now := l.now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)

	entry, ok := l.entries[key]
	if !ok || !now.Before(entry.resetAt) {
		resetAt := now.Add(l.window)
		l.entries[key] = rateLimitEntry{count: 1, resetAt: resetAt, lastSeen: now}
		return true, resetAt
	}

	entry.lastSeen = now
	if entry.count >= l.requests {
		l.entries[key] = entry
		return false, entry.resetAt
	}

	entry.count++
	l.entries[key] = entry
	return true, entry.resetAt
}

func (l *RateLimiter) prune(now time.Time) {
	for key, entry := range l.entries {
		if now.After(entry.resetAt.Add(l.window)) {
			delete(l.entries, key)
		}
	}
}

func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || shouldSkipRateLimit(r) {
				next.ServeHTTP(w, r)
				return
			}

			key := rateLimitKey(r)
			allowed, resetAt := limiter.Allow(key)
			if !allowed {
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":"TOO_MANY_REQUESTS","message":"Rate limit exceeded"}}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func shouldSkipRateLimit(r *http.Request) bool {
	if r.Method == http.MethodOptions {
		return true
	}
	switch r.URL.Path {
	case "/api/v1/health", "/api/v1/readiness", "/api/v1/github/webhook", "/api/v1/webhooks/github":
		return true
	default:
		return false
	}
}

func rateLimitKey(r *http.Request) string {
	if principal, ok := auth.PrincipalFromContext(r.Context()); ok && strings.TrimSpace(principal.User.ID) != "" {
		return "user:" + principal.User.ID
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return "ip:" + host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return "ip:" + strings.TrimSpace(r.RemoteAddr)
	}
	return "ip:unknown"
}
