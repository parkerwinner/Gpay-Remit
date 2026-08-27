package chaos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Chaos scenarios: database failure, network partition, and service down.
//
// The distinction these turn on is between a dependency that *fails* and one
// that *hangs*. A refused connection returns in microseconds and any caller
// survives it; a partition blocks until something times out, and a caller with
// no deadline waits forever. Most of the cases below exist to prove code takes
// a context and honours it.

const testSeed = 1

func TestHealthyInjectorPassesThrough(t *testing.T) {
	inj := NewInjector(testSeed)
	called := false

	err := inj.Do(context.Background(), func() error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, int64(1), inj.Calls())
	assert.Equal(t, int64(0), inj.Failures())
}

func TestHealthyInjectorPropagatesTheOperationError(t *testing.T) {
	// The injector must not swallow or rewrite a genuine application error.
	inj := NewInjector(testSeed)
	sentinel := errors.New("real failure")

	err := inj.Do(context.Background(), func() error { return sentinel })

	assert.ErrorIs(t, err, sentinel)
}

// ── Scenario: dependency is down ────────────────────────────────────────────

func TestDatabaseDownRefusesImmediately(t *testing.T) {
	inj := NewInjector(testSeed)
	inj.SetMode(ModeDown)

	called := false
	start := time.Now()
	err := inj.Do(context.Background(), func() error {
		called = true
		return nil
	})

	assert.ErrorIs(t, err, ErrConnectionRefused)
	// The operation must never run — a refused connection means the query was
	// never issued.
	assert.False(t, called)
	// And it fails fast rather than blocking, which is what distinguishes it
	// from a partition.
	assert.Less(t, time.Since(start), 50*time.Millisecond)
}

func TestServiceDownIsCountedAsAFailure(t *testing.T) {
	inj := NewInjector(testSeed)
	inj.SetMode(ModeDown)

	for i := 0; i < 5; i++ {
		_ = inj.Do(context.Background(), func() error { return nil })
	}

	assert.Equal(t, int64(5), inj.Calls())
	assert.Equal(t, int64(5), inj.Failures())
}

func TestUnreachableServerRefusesConnections(t *testing.T) {
	url, err := NewUnreachableServer()
	require.NoError(t, err)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}

	// A genuinely closed port, so this is the OS refusing rather than a
	// synthetic error the code might handle differently.
	require.Error(t, err)
}

// ── Scenario: network partition ─────────────────────────────────────────────

func TestPartitionBlocksUntilTheContextExpires(t *testing.T) {
	inj := NewInjector(testSeed)
	inj.SetMode(ModePartition)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := inj.Do(ctx, func() error { return nil })
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, ErrPartitioned)
	// It waited rather than failing fast — this is the mode that finds callers
	// with no deadline.
	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
}

func TestPartitionIsSurvivableOnlyWithADeadline(t *testing.T) {
	// The point of the scenario: identical code, one with a deadline and one
	// without, behave completely differently under partition. Only the first
	// returns at all.
	inj := NewInjector(testSeed)
	inj.SetMode(ModePartition)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- inj.Do(ctx, func() error { return nil }) }()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, ErrPartitioned)
	case <-time.After(2 * time.Second):
		t.Fatal("call with a deadline never returned under partition")
	}
}

func TestBlackholeServerAcceptsThenNeverAnswers(t *testing.T) {
	url, closeFn := NewBlackholeServer()
	defer closeFn()

	// Distinct from an unreachable server: the handshake succeeds, so the
	// client commits and waits. Only its own timeout ends the call.
	client := &http.Client{Timeout: 100 * time.Millisecond}
	start := time.Now()
	resp, err := client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}

	require.Error(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
}

func TestBlackholeServerCloseIsIdempotent(t *testing.T) {
	_, closeFn := NewBlackholeServer()
	closeFn()
	// A double close would panic on the already-closed channel; deferred
	// cleanup plus an explicit close is a normal pattern in tests.
	assert.NotPanics(t, closeFn)
}

// ── Scenario: degraded service ──────────────────────────────────────────────

func TestSlowModeEventuallySucceeds(t *testing.T) {
	inj := NewInjector(testSeed)
	inj.SetMode(ModeSlow)
	inj.SetLatency(30 * time.Millisecond)

	start := time.Now()
	err := inj.Do(context.Background(), func() error { return nil })

	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 30*time.Millisecond)
}

func TestSlowModeStillHonoursAShorterDeadline(t *testing.T) {
	// Degradation must not defeat a caller's own timeout budget.
	inj := NewInjector(testSeed)
	inj.SetMode(ModeSlow)
	inj.SetLatency(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := inj.Do(ctx, func() error { return nil })

	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestFlakyModeFailsSomeCallsAndSucceedsOthers(t *testing.T) {
	inj := NewInjector(testSeed)
	inj.SetMode(ModeFlaky)
	inj.SetFailureRate(0.5)

	for i := 0; i < 200; i++ {
		_ = inj.Do(context.Background(), func() error { return nil })
	}

	failures := inj.Failures()
	// Not asserting an exact count — the seed makes it reproducible, but the
	// property that matters is that the mode is genuinely intermittent rather
	// than all-or-nothing.
	assert.Greater(t, failures, int64(0))
	assert.Less(t, failures, int64(200))
}

func TestFlakyModeIsReproducibleForAGivenSeed(t *testing.T) {
	run := func() int64 {
		inj := NewInjector(99)
		inj.SetMode(ModeFlaky)
		inj.SetFailureRate(0.3)
		for i := 0; i < 100; i++ {
			_ = inj.Do(context.Background(), func() error { return nil })
		}
		return inj.Failures()
	}

	// A chaos failure nobody can reproduce is a chaos failure nobody fixes.
	assert.Equal(t, run(), run())
}

func TestFailureRateIsClamped(t *testing.T) {
	inj := NewInjector(testSeed)
	inj.SetMode(ModeFlaky)

	inj.SetFailureRate(5)
	for i := 0; i < 20; i++ {
		_ = inj.Do(context.Background(), func() error { return nil })
	}
	assert.Equal(t, int64(20), inj.Failures(), "a rate above 1 should fail everything")

	inj2 := NewInjector(testSeed)
	inj2.SetMode(ModeFlaky)
	inj2.SetFailureRate(-1)
	for i := 0; i < 20; i++ {
		_ = inj2.Do(context.Background(), func() error { return nil })
	}
	assert.Equal(t, int64(0), inj2.Failures(), "a negative rate should fail nothing")
}

func TestFailingServerReturnsTheInjectedStatus(t *testing.T) {
	server := NewFailingServer(http.StatusInternalServerError)
	defer server.Close()

	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "chaos")
}

func TestSlowServerRespectsClientCancellation(t *testing.T) {
	server := NewSlowServer(2*time.Second, http.StatusOK)
	defer server.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Get(server.URL)
	if resp != nil {
		defer resp.Body.Close()
	}

	require.Error(t, err)
}

// ── Scenario: recovery ──────────────────────────────────────────────────────

func TestIntermittentServerRecoversAfterTheConfiguredFailures(t *testing.T) {
	server, attempts := NewIntermittentServer(2)
	defer server.Close()

	// Two failures, then success — this is what retry logic has to converge on.
	var last int
	for i := 0; i < 3; i++ {
		resp, err := http.Get(server.URL)
		require.NoError(t, err)
		last = resp.StatusCode
		resp.Body.Close()
	}

	assert.Equal(t, http.StatusOK, last)
	assert.Equal(t, int64(3), attempts())
}

func TestInjectorRecoversWhenTheModeIsRestored(t *testing.T) {
	// Recovery matters as much as failure: a circuit that opens and never
	// closes is its own outage.
	inj := NewInjector(testSeed)
	inj.SetMode(ModeDown)
	require.ErrorIs(t, inj.Do(context.Background(), func() error { return nil }), ErrConnectionRefused)

	inj.SetMode(ModeHealthy)
	assert.NoError(t, inj.Do(context.Background(), func() error { return nil }))
}

func TestModeCanBeChangedWhileCallsAreInFlight(t *testing.T) {
	// A real outage starts mid-request. Run under -race, this also proves the
	// injector's own locking is sound.
	inj := NewInjector(testSeed)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_ = inj.Do(ctx, func() error { return nil })
		}()
	}

	for _, mode := range []Mode{ModeDown, ModeSlow, ModeHealthy} {
		inj.SetMode(mode)
		time.Sleep(2 * time.Millisecond)
	}
	wg.Wait()

	assert.Equal(t, int64(20), inj.Calls())
}

func TestResetClearsCountersButKeepsTheMode(t *testing.T) {
	inj := NewInjector(testSeed)
	inj.SetMode(ModeDown)
	_ = inj.Do(context.Background(), func() error { return nil })

	inj.Reset()

	assert.Equal(t, int64(0), inj.Calls())
	assert.Equal(t, int64(0), inj.Failures())
	assert.Equal(t, ModeDown, inj.Mode())
}

func TestModeStringsAreReadable(t *testing.T) {
	// These end up in test failure output, so they need to name the scenario.
	assert.Equal(t, "healthy", ModeHealthy.String())
	assert.Equal(t, "down", ModeDown.String())
	assert.Equal(t, "partition", ModePartition.String())
	assert.Equal(t, "slow", ModeSlow.String())
	assert.Equal(t, "flaky", ModeFlaky.String())
	assert.Equal(t, "unknown", Mode(99).String())
}
