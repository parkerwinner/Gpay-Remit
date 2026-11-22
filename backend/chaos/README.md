# Chaos & Resilience Testing

This directory implements fault-injection and chaos engineering test scenarios for Gpay-Remit.

---

## 1. Overview

Distributed financial applications must tolerate network latency, database disruptions, and third-party API partitions gracefully without losing funds, hanging indefinitely, or corrupting state.

The chaos suite verifies two categories of failure modes:
1. **Immediate Failures**: A service or database connection is refused or reset. Callers must fail fast, return meaningful errors, or retry with backoff.
2. **Hanging/Slow Failures (Partitions)**: A connection accepts packets but never responds, or injects high latency. Callers must honor context deadlines and abort rather than leak goroutines.

---

## 2. Test Suites

### 2.1 In-Process Chaos Suite (`chaos_test.go`)
- **Execution**: Deterministic, memory-based, runs without Docker.
- **Components**:
  - `Injector`: Configurable fault injector with controllable failure rate, artificial latency, and jitter.
  - Context cancellation & deadline honoring.
  - Concurrency safety under high contention.
- **Run Locally**:
  ```bash
  cd backend
  go test -race -v ./chaos/...
  ```

### 2.2 Toxiproxy Suite (`toxiproxy_test.go`)
- **Execution**: Real TCP proxy fault injection via Shopify's [Toxiproxy](https://github.com/shopify/toxiproxy).
- **Failure Scenarios**:
  - `latency`: Simulates cross-region latency spikes and packet delays.
  - `down`: Simulates complete network partitions and socket resets.
  - `bandwidth`: Throttles throughput to verify slow client handling.
  - `slicer`: Slices TCP packets into tiny fragments to test stream reassembly.
- **Run Locally**:
  ```bash
  docker run -d --name toxiproxy -p 8474:8474 -p 25432:25432 ghcr.io/shopify/toxiproxy:latest
  cd backend
  TOXIPROXY=1 go test -race -v -tags=toxiproxy ./chaos/...
  ```

---

## 3. CI Nightly Workflow

The chaos suite runs automatically every night at 04:00 UTC via `.github/workflows/chaos-nightly.yml`.
- Runs both in-process and Toxiproxy test suites.
- Employs `TOXIPROXY=1` flag with a Toxiproxy service container.
- Alerts on failure by notifying the on-call channel.
