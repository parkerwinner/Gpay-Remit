# Runbook: Stellar Horizon Outage

## Severity
- **SEV-1**

## Symptoms
- Alert: `StellarHorizonUnreachable` or transaction submissions failing with network timeout.
- Backend logs show: `horizon client: request timed out`, `504 Gateway Timeout from Horizon`, `Stellar ledger lag detected`.
- Remittance creation fails or hangs in `pending` state.

## Diagnostic Steps
1. Test Horizon endpoint reachability directly:
   ```bash
   curl -I https://horizon.stellar.org/
   # Or for testnet:
   curl -I https://horizon-testnet.stellar.org/
   ```
2. Check official Stellar status page:
   - https://status.stellar.org/
3. Verify ledger sync status:
   ```bash
   curl -s https://horizon.stellar.org/fee_stats | jq .
   ```

## Mitigation & Recovery
1. **Switch to Backup Horizon RPC Node**:
   - Update `STELLAR_HORIZON_URL` in environment configuration to secondary/backup provider (e.g. public anchor node or private Horizon instance):
     ```bash
     export STELLAR_HORIZON_URL=https://stellar-horizon-backup.example.com
     ```
   - Restart the backend API pods to apply configuration.
2. **Handle In-Flight Escrows**:
   - Query remittances stuck in `pending` state:
     ```sql
     SELECT id, tx_hash, created_at FROM remittances WHERE status = 'pending' AND created_at < NOW() - INTERVAL '10 minutes';
     ```
   - Resubmit transactions once network connectivity recovers using the idempotent transaction envelope.
