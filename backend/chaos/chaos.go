// Package chaos provides fault injection for testing the system under failure
// conditions.
//
// The failures worth testing are not "does the code return an error" — they are
// the ones where a dependency behaves *badly* rather than being cleanly absent:
//
//   - **Database down.** Connections refused outright.
//   - **Network partition.** Connections that hang rather than fail, which is
//     the dangerous case: a refused connection returns in microseconds, a
//     partitioned one blocks until something times out. Code without a timeout
//     survives the first and hangs forever on the second.
//   - **Service degraded.** Responses that arrive, but slowly or wrongly —
//     latency spikes, intermittent 500s, truncated bodies.
//
// ── Why not Toxiproxy by default ────────────────────────────────────────────
//
// Toxiproxy is the right tool for end-to-end chaos against real infrastructure,
// and `chaos_toxiproxy_test.go` uses it. But it needs a running proxy and Docker,
// which means those tests cannot run on a machine without them — and a chaos
// suite that is usually skipped provides no protection.
//
// So the default injectors here are in-process and deterministic: they need no
// Docker, run in milliseconds, and fail reproducibly. The Toxiproxy suite sits
// behind a build tag for when real infrastructure is available.
package chaos

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

// ErrConnectionRefused mimics a dependency that is down.
var ErrConnectionRefused = errors.New("chaos: connection refused")

// ErrPartitioned mimics a network partition where the peer never answers.
var ErrPartitioned = errors.New("chaos: network partition (deadline exceeded)")

// Mode selects the failure a Injector produces.
type Mode int

const (
	// ModeHealthy passes calls through untouched.
	ModeHealthy Mode = iota
	// ModeDown refuses every call immediately.
	ModeDown
	// ModePartition blocks until the caller's context expires. This is the mode
	// that finds missing timeouts.
	ModePartition
	// ModeSlow delays each call but eventually succeeds.
	ModeSlow
	// ModeFlaky fails a configurable fraction of calls.
	ModeFlaky
)

func (m Mode) String() string {
	switch m {
	case ModeHealthy:
		return "healthy"
	case ModeDown:
		return "down"
	case ModePartition:
		return "partition"
	case ModeSlow:
		return "slow"
	case ModeFlaky:
		return "flaky"
	default:
		return "unknown"
	}
}

// Injector applies a failure mode to an operation.
//
// Safe for concurrent use: mode changes are guarded so a test can flip a
// dependency to ModeDown while requests are already in flight, which is what
// a real outage looks like.
type Injector struct {
	mu sync.RWMutex

	mode Mode
	// Latency added in ModeSlow.
	latency time.Duration
	// Failure probability in ModeFlaky, 0..1.
	failureRate float64
	// Deterministic source so a flaky run can be replayed.
	rng *rand.Rand

	calls    atomic.Int64
	failures atomic.Int64
}

// NewInjector returns a healthy injector seeded for reproducibility.
func NewInjector(seed int64) *Injector {
	return &Injector{
		mode:        ModeHealthy,
		latency:     100 * time.Millisecond,
		failureRate: 0.5,
		rng:         rand.New(rand.NewSource(seed)),
	}
}

// SetMode switches the failure mode. Safe to call while calls are in flight.
func (i *Injector) SetMode(mode Mode) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.mode = mode
}

// SetLatency configures the delay used by ModeSlow.
func (i *Injector) SetLatency(d time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.latency = d
}

// SetFailureRate configures the failure probability used by ModeFlaky.
func (i *Injector) SetFailureRate(rate float64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	switch {
	case rate < 0:
		i.failureRate = 0
	case rate > 1:
		i.failureRate = 1
	default:
		i.failureRate = rate
	}
}

// Mode reports the current mode.
func (i *Injector) Mode() Mode {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.mode
}

// Calls returns how many operations passed through the injector.
func (i *Injector) Calls() int64 { return i.calls.Load() }

// Failures returns how many were failed by the injector.
func (i *Injector) Failures() int64 { return i.failures.Load() }

// Reset clears the counters without changing the mode.
func (i *Injector) Reset() {
	i.calls.Store(0)
	i.failures.Store(0)
}

// Do runs op under the current failure mode.
//
// ModePartition deliberately waits on ctx.Done() rather than returning an
// error straight away. An operation invoked with context.Background() will
// block forever here — which is the point: that is precisely what happens in
// production, and a test that hangs has found a missing timeout.
func (i *Injector) Do(ctx context.Context, op func() error) error {
	i.calls.Add(1)

	i.mu.RLock()
	mode, latency, rate := i.mode, i.latency, i.failureRate
	i.mu.RUnlock()

	switch mode {
	case ModeDown:
		i.failures.Add(1)
		return ErrConnectionRefused

	case ModePartition:
		<-ctx.Done()
		i.failures.Add(1)
		return fmt.Errorf("%w: %v", ErrPartitioned, ctx.Err())

	case ModeSlow:
		select {
		case <-time.After(latency):
		case <-ctx.Done():
			i.failures.Add(1)
			return ctx.Err()
		}

	case ModeFlaky:
		i.mu.Lock()
		shouldFail := i.rng.Float64() < rate
		i.mu.Unlock()
		if shouldFail {
			i.failures.Add(1)
			return ErrConnectionRefused
		}
	}

	if err := ctx.Err(); err != nil {
		i.failures.Add(1)
		return err
	}
	return op()
}

// ── HTTP fault injection ────────────────────────────────────────────────────

// NewUnreachableServer returns a URL that refuses connections.
//
// Built by starting a listener and closing it, so the port is genuinely closed
// rather than merely unrouted — the caller sees a real connection refusal from
// the OS instead of a synthetic error.
func NewUnreachableServer() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", err
	}
	return "http://" + addr, nil
}

// NewBlackholeServer returns a server that accepts connections and never
// responds, plus a close function.
//
// This is the network-partition case at the HTTP layer, and it is distinct from
// an unreachable server: the TCP handshake succeeds, so the client commits to
// the request and waits. Only a client-side timeout ends it.
func NewBlackholeServer() (url string, closeFn func()) {
	blocked := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-blocked:
		case <-r.Context().Done():
		}
	}))
	var once sync.Once
	return server.URL, func() {
		once.Do(func() {
			close(blocked)
			server.Close()
		})
	}
}

// NewFailingServer returns a server that responds with the given status code.
func NewFailingServer(status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"chaos: injected failure"}`))
	}))
}

// NewSlowServer returns a server that waits before responding.
func NewSlowServer(delay time.Duration, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"status":"slow but ok"}`))
		case <-r.Context().Done():
		}
	}))
}

// NewIntermittentServer fails the first failuresBeforeSuccess requests, then
// succeeds. Used to check that retry logic actually converges rather than
// giving up or retrying forever.
func NewIntermittentServer(failuresBeforeSuccess int) (*httptest.Server, func() int64) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n <= int64(failuresBeforeSuccess) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"chaos: temporary failure"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"recovered"}`))
	}))
	return server, attempts.Load
}
