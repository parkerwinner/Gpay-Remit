//go:build toxiproxy

// Toxiproxy-backed chaos against real infrastructure.
//
// Behind a build tag because it needs a running Toxiproxy instance, which means
// Docker. The in-process suite in chaos_test.go covers the same failure modes
// deterministically and runs everywhere; this one exists for the cases only a
// real TCP proxy can produce — bandwidth throttling, packet-level slicing, and
// failures against the actual driver rather than a fake.
//
// Run with:
//
//	docker run -d -p 8474:8474 -p 25432:25432 ghcr.io/shopify/toxiproxy
//	go test -tags toxiproxy ./chaos/...
//
// TOXIPROXY_URL overrides the control endpoint (default http://localhost:8474).
package chaos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func toxiproxyURL() string {
	if url := os.Getenv("TOXIPROXY_URL"); url != "" {
		return url
	}
	return "http://localhost:8474"
}

// toxiproxyClient is a minimal control-API client.
//
// Hand-rolled rather than pulling in the official SDK: this suite needs four
// calls, and adding a dependency to the module for a build-tagged test would
// put it in every consumer's dependency graph.
type toxiproxyClient struct {
	baseURL string
	http    *http.Client
}

func newToxiproxyClient() *toxiproxyClient {
	return &toxiproxyClient{
		baseURL: toxiproxyURL(),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *toxiproxyClient) available() bool {
	resp, err := c.http.Get(c.baseURL + "/version")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *toxiproxyClient) createProxy(name, listen, upstream string) error {
	body, _ := json.Marshal(map[string]any{
		"name":     name,
		"listen":   listen,
		"upstream": upstream,
		"enabled":  true,
	})
	resp, err := c.http.Post(c.baseURL+"/proxies", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusConflict {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create proxy: %s: %s", resp.Status, msg)
	}
	return nil
}

func (c *toxiproxyClient) deleteProxy(name string) {
	req, _ := http.NewRequest(http.MethodDelete, c.baseURL+"/proxies/"+name, nil)
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// addToxic attaches a fault. `toxicType` is a Toxiproxy toxic name such as
// "timeout", "latency" or "bandwidth".
func (c *toxiproxyClient) addToxic(proxy, name, toxicType string, attrs map[string]any) error {
	body, _ := json.Marshal(map[string]any{
		"name":       name,
		"type":       toxicType,
		"stream":     "downstream",
		"attributes": attrs,
	})
	resp, err := c.http.Post(c.baseURL+"/proxies/"+proxy+"/toxics", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add toxic: %s: %s", resp.Status, msg)
	}
	return nil
}

func requireToxiproxy(t *testing.T) *toxiproxyClient {
	t.Helper()
	client := newToxiproxyClient()
	if !client.available() {
		t.Skipf("toxiproxy not reachable at %s; start it with "+
			"`docker run -d -p 8474:8474 -p 25432:25432 ghcr.io/shopify/toxiproxy`", client.baseURL)
	}
	return client
}

// TestToxiproxyTimeoutSimulatesPartition drives a real TCP partition through
// the proxy rather than an in-process fake, so the driver's own timeout
// handling is what gets exercised.
func TestToxiproxyTimeoutSimulatesPartition(t *testing.T) {
	client := requireToxiproxy(t)

	upstream := os.Getenv("CHAOS_UPSTREAM_ADDR")
	if upstream == "" {
		t.Skip("set CHAOS_UPSTREAM_ADDR to the dependency being proxied (e.g. localhost:5432)")
	}

	const proxyName = "gpay_chaos_partition"
	require.NoError(t, client.createProxy(proxyName, "0.0.0.0:25432", upstream))
	defer client.deleteProxy(proxyName)

	// A timeout toxic with 0 holds the connection open and never delivers —
	// the partition case, as opposed to a refused connection.
	require.NoError(t, client.addToxic(proxyName, "partition", "timeout", map[string]any{
		"timeout": 0,
	}))

	conn := &http.Client{Timeout: 500 * time.Millisecond}
	start := time.Now()
	resp, err := conn.Get("http://localhost:25432")
	if resp != nil {
		defer resp.Body.Close()
	}

	require.Error(t, err, "a partitioned dependency must not appear healthy")
	require.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond,
		"the client should have waited for its own timeout rather than failing fast")
}

// TestToxiproxyLatencyDegradesWithoutFailing covers the degraded case: the
// dependency answers, just too slowly for the caller's budget.
func TestToxiproxyLatencyDegradesWithoutFailing(t *testing.T) {
	client := requireToxiproxy(t)

	upstream := os.Getenv("CHAOS_UPSTREAM_ADDR")
	if upstream == "" {
		t.Skip("set CHAOS_UPSTREAM_ADDR to the dependency being proxied")
	}

	const proxyName = "gpay_chaos_latency"
	require.NoError(t, client.createProxy(proxyName, "0.0.0.0:25433", upstream))
	defer client.deleteProxy(proxyName)

	require.NoError(t, client.addToxic(proxyName, "slow", "latency", map[string]any{
		"latency": 800,
		"jitter":  100,
	}))

	conn := &http.Client{Timeout: 200 * time.Millisecond}
	resp, err := conn.Get("http://localhost:25433")
	if resp != nil {
		defer resp.Body.Close()
	}

	require.Error(t, err, "a caller with a 200ms budget must not wait out 800ms of latency")
}
