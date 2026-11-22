# Runbook: Redis Unavailable

## Severity
- **SEV-2**

## Symptoms
- Alert: `RedisConnectionFailure` or elevated latency across rate-limited endpoints.
- Backend logs show: `dial tcp: connection refused (redis)`, `OOM command not allowed when used memory > 'maxmemory'`.
- Exchange rate caching falls back to database or direct oracle calls.

## Diagnostic Steps
1. Verify Redis process and responsiveness:
   ```bash
   redis-cli ping
   # Expected: PONG
   ```
2. Inspect memory usage:
   ```bash
   redis-cli info memory
   ```
3. Check container/pod logs:
   ```bash
   docker compose logs --tail=100 redis
   ```

## Mitigation & Recovery
1. **Restart Instance**:
   ```bash
   docker compose restart redis
   ```
2. **Handle Eviction Pressure**:
   - If memory limit is breached, verify eviction policy:
     ```bash
     redis-cli config get maxmemory-policy
     # Should be: volatile-lru or allkeys-lru
     ```
   - Flush non-essential cache keys if system is jammed:
     ```bash
     redis-cli --scan --pattern "cache:rates:*" | xargs redis-cli del
     ```
3. **Verify Restoration**:
   - Check Prometheus metric `gpay_remit_redis_hits_total` is increasing.
