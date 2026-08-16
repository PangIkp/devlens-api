package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeRotatingStore mimics the real Postgres RotateSession's compare-and-swap
// semantics: rotation only succeeds if refresh_token_hash still matches what
// the caller originally read. A barrier holds every concurrent
// GetSessionByRefreshTokenHash call until all of them have observed the same
// (pre-rotation) hash, forcing a genuine race on the subsequent RotateSession
// calls instead of letting goroutines serialize by luck.
type fakeRotatingStore struct {
	mu               sync.Mutex
	sessionID        string
	refreshTokenHash string
	refreshExpiresAt time.Time
	rotateCalls      int

	barrierSize int
	getCalls    int
	barrier     chan struct{}
}

func (f *fakeRotatingStore) CreateOrUpdateUser(context.Context, string, *string) (User, error) {
	return User{}, ErrSessionNotFound
}

func (f *fakeRotatingStore) CreateSession(context.Context, createSessionParams) error {
	return ErrSessionNotFound
}

func (f *fakeRotatingStore) GetSessionByAccessTokenHash(context.Context, string) (sessionRecord, error) {
	return sessionRecord{}, ErrSessionNotFound
}

func (f *fakeRotatingStore) GetSessionByRefreshTokenHash(_ context.Context, hash string) (sessionRecord, error) {
	f.mu.Lock()
	if hash != f.refreshTokenHash {
		f.mu.Unlock()
		return sessionRecord{}, ErrSessionNotFound
	}
	f.getCalls++
	ready := f.getCalls == f.barrierSize
	record := sessionRecord{
		ID:               f.sessionID,
		RefreshTokenHash: f.refreshTokenHash,
		RefreshExpiresAt: f.refreshExpiresAt,
		User:             User{ID: "user-1", Email: "user@example.com"},
	}
	f.mu.Unlock()

	if ready {
		close(f.barrier)
	} else {
		<-f.barrier
	}
	return record, nil
}

func (f *fakeRotatingStore) TouchSession(context.Context, string, time.Time) error {
	return nil
}

func (f *fakeRotatingStore) RotateSession(_ context.Context, params rotateSessionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rotateCalls++
	if f.refreshTokenHash != params.OldRefreshTokenHash {
		return ErrSessionNotFound
	}
	f.refreshTokenHash = params.RefreshTokenHash
	return nil
}

func (f *fakeRotatingStore) RevokeSession(context.Context, string, time.Time) error {
	return nil
}

func TestRefreshConcurrentCallsWithSameTokenOnlyOneSucceeds(t *testing.T) {
	t.Parallel()

	const attempts = 5
	store := &fakeRotatingStore{
		sessionID:        "session-1",
		refreshTokenHash: tokenHash("initial-refresh-token"),
		refreshExpiresAt: time.Now().UTC().Add(time.Hour),
		barrierSize:      attempts,
		barrier:          make(chan struct{}),
	}
	svc := NewService(store, time.Minute, time.Hour)

	var wg sync.WaitGroup
	results := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: "initial-refresh-token"})
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful refresh out of %d concurrent calls sharing one refresh token, got %d", attempts, successes)
	}
	if store.rotateCalls != attempts {
		t.Fatalf("expected all %d attempts to reach RotateSession, got %d", attempts, store.rotateCalls)
	}
}
