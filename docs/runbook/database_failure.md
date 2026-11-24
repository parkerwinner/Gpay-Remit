# Runbook: Database Connection Failure

## Severity
- **SEV-1** (if complete outage)
- **SEV-2** (if intermittent pool exhaustion)

## Symptoms
- Alert: `DatabaseDown` or high rate of `500 Internal Server Error`.
- Backend logs show: `pq: the database system is shutting down`, `dial tcp: connection refused`, or `database connection pool exhausted`.
- Health probe `GET /health/ready` returns `503 Service Unavailable`.

## Diagnostic Steps
1. Verify database process status:
   ```bash
   # If running under Docker Compose:
   docker compose ps postgres
   docker compose logs --tail=100 postgres

   # If running in Kubernetes:
   kubectl get pods -l app=postgres
   kubectl logs -l app=postgres --tail=100
   ```
2. Test network connectivity from backend to DB:
   ```bash
   pg_isready -h localhost -p 5432 -U gpay
   ```
3. Check active connections vs `max_connections`:
   ```sql
   SELECT count(*), state FROM pg_stat_activity GROUP BY state;
   SHOW max_connections;
   ```

## Mitigation & Recovery
1. **Restart or Failover**:
   - For container failure, restart the PostgreSQL instance:
     ```bash
     docker compose restart postgres
     ```
   - In production with replicas, initiate an automated or manual replica promotion if primary is dead.
2. **Pool Exhaustion Resolution**:
   - If queries are hanging, inspect blocking transactions:
     ```sql
     SELECT pid, query, age(clock_timestamp(), query_start)
     FROM pg_stat_activity
     WHERE query != '<IDLE>' AND age(clock_timestamp(), query_start) > interval '30 seconds';
     ```
   - Terminate rogue connection:
     ```sql
     SELECT pg_terminate_backend(<pid>);
     ```
3. **Verify Restoration**:
   - Check `GET /health/ready` -> expects HTTP 200 OK.
