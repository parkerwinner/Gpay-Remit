package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/config"
)

// These tests build their own RateLimiter (via newRateLimiter, not the
// process-wide GetRateLimiter singleton) with short cleanupInterval/
// staleAfter values, so the cleanup goroutine's actual behavior can be
// observed deterministically in test time rather than waiting on the
// production 5-minute/1-hour defaults.

func TestRateLimiter_CleanupRemovesStaleEntries(t *testing.T) {
	rl := newRateLimiter(&config.Config{}, 20*time.Millisecond, 60*time.Millisecond)
	defer rl.Stop()

	rl.IncrementAndCheck("stale-key", 10, time.Minute)
	assert.NotNil(t, rl.GetLimit("stale-key"), "entry should exist immediately after creation")

	// Past staleAfter, and at least one cleanup tick has had time to run.
	time.Sleep(80 * time.Millisecond)

	assert.Nil(t, rl.GetLimit("stale-key"), "cleanup should have removed the entry once it exceeded staleAfter")
}

func TestRateLimiter_CleanupKeepsRecentlyAccessedEntries(t *testing.T) {
	rl := newRateLimiter(&config.Config{}, 20*time.Millisecond, 200*time.Millisecond)
	defer rl.Stop()

	rl.IncrementAndCheck("active-key", 10, time.Minute)

	// A cleanup tick runs well before staleAfter elapses; the entry must
	// survive since it hasn't gone unaccessed for staleAfter yet.
	time.Sleep(60 * time.Millisecond)

	assert.NotNil(t, rl.GetLimit("active-key"), "cleanup must not remove entries accessed within staleAfter")
}

func TestRateLimiter_CleanupOnlyRemovesTheStaleEntry(t *testing.T) {
	rl := newRateLimiter(&config.Config{}, 20*time.Millisecond, 60*time.Millisecond)
	defer rl.Stop()

	rl.IncrementAndCheck("will-go-stale", 10, time.Minute)

	// Give "will-go-stale" time to become stale, then touch a second key
	// right before the assertion so it's fresh.
	time.Sleep(70 * time.Millisecond)
	rl.IncrementAndCheck("stays-fresh", 10, time.Minute)
	time.Sleep(20 * time.Millisecond)

	assert.Nil(t, rl.GetLimit("will-go-stale"), "the genuinely stale key should be removed")
	assert.NotNil(t, rl.GetLimit("stays-fresh"), "a key accessed just before the sweep must survive it")
}

func TestRateLimiter_StopTerminatesCleanupGoroutine(t *testing.T) {
	rl := newRateLimiter(&config.Config{}, 5*time.Millisecond, time.Hour)

	done := make(chan struct{})
	go func() {
		rl.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop() returned (channel close doesn't block on the goroutine
		// exiting, but this at least confirms Stop() itself doesn't hang
		// or panic on a fresh, running limiter).
	case <-time.After(time.Second):
		t.Fatal("Stop() did not return in time")
	}
}

func TestDefaultRateLimiter_UsesDocumentedIntervals(t *testing.T) {
	// Pins #196's acceptance criteria ("run cleanup every 5 minutes")
	// against a regression that silently changes the production defaults.
	assert.Equal(t, 5*time.Minute, defaultCleanupInterval)
	assert.Equal(t, 1*time.Hour, defaultStaleAfter)
}
