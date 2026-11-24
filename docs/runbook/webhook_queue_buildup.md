# Runbook: Webhook Delivery Queue Buildup

## Severity
- **SEV-2**

## Symptoms
- Alert: `WebhookQueueBacklog` or high rate of `webhook_retry_attempts_total`.
- Prometheus metric `webhook_dead_letters_total` increasing rapidly.
- Users report missing or delayed transaction notification callbacks.

## Diagnostic Steps
1. Query count of pending and failed deliveries:
   ```sql
   SELECT status, count(*) FROM webhook_deliveries GROUP BY status;
   ```
2. Identify failing webhook endpoints and error codes:
   ```sql
   SELECT w.url, d.response_code, d.error_message, count(*)
   FROM webhook_deliveries d
   JOIN webhooks w ON w.id = d.webhook_id
   WHERE d.status = 'failed'
   GROUP BY w.url, d.response_code, d.error_message
   ORDER BY count(*) DESC
   LIMIT 10;
   ```
3. Check webhook retry worker status:
   ```bash
   docker compose logs --tail=100 backend | grep -i "webhook"
   ```

## Mitigation & Recovery
1. **Disable Bad Consumer Endpoints**:
   - If a specific consumer URL is returning 500s or timing out and clogging worker threads, temporarily disable that webhook:
     ```sql
     UPDATE webhooks SET is_active = false WHERE id = <offending_webhook_id>;
     ```
2. **Replay Dead-Letter Deliveries**:
   - Once consumer fixes their server, trigger replay via API:
     ```bash
     curl -X POST -H "Authorization: Bearer <TOKEN>" \
       https://api.gpay-remit.example.com/api/v1/webhooks/deliveries/<delivery_id>/retry
     ```
3. **Scale Webhook Worker**:
   - If system-wide volume spiked, scale worker concurrency or increase database connection limit.
