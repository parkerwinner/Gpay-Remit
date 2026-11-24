# Performance and cache monitoring

## Contract benchmarks

Contract benchmarks run on pull requests and pushes to `main`. The first pull-request run measures its base branch when no cached baseline exists; later runs restore the latest `main` Criterion baseline. Pull requests fail when a benchmark mean is more than 10% slower than its baseline. Results are uploaded as the `contract-benchmarks-*` artifact for 30 days.

## Staging load tests

The weekly and release-branch k6 workflow requires these repository secrets:

- `STAGING_BASE_URL`: staging origin, without `/api/v1` (for example, `https://staging.example.com`).
- `K6_TEST_EMAIL` and `K6_TEST_PASSWORD`: a seeded staging account that can read remittances.

The workflow checks readiness and authenticated remittance reads. The p99 read latency SLO is 500 ms; a failed check or threshold fails the workflow.

## Redis cache

Docker Compose starts Redis with `maxmemory` set to `REDIS_MAXMEMORY` (default `256mb`) and `allkeys-lru`. Cache writes made through `SetCached` use a five-minute TTL when the caller supplies a non-positive TTL; explicit positive TTLs are preserved.

The API exports `gpay_remit_cache_operations_total` and `gpay_remit_redis_evicted_keys_total` at `/metrics`. Load `monitoring/redis-cache-alerts.yml` in Prometheus to alert when the eviction counter increases; `rate(gpay_remit_redis_evicted_keys_total[5m])` reports the eviction rate.
