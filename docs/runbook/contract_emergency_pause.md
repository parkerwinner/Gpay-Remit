# Runbook: Contract Emergency Pause

## Severity
- **SEV-1** (if unintended pause) or **Informational/Active Incident Response** (if manual pause due to security threat)

## Symptoms
- On-chain Soroban contract invocations fail with error code `ContractPaused` or `EmergencyPauseActive`.
- API endpoints attempting escrow deposits or releases return 400 with message `contract operations temporarily paused`.

## Diagnostic Steps
1. Query the on-chain contract pause status:
   ```bash
   soroban contract invoke \
     --id <PAYMENT_ESCROW_CONTRACT_ID> \
     --network testnet \
     --source <ADMIN_ACCOUNT> \
     -- is_paused
   ```
2. Inspect audit events and logs to identify who executed `pause()`:
   ```sql
   SELECT * FROM audit_logs WHERE action = 'contract.emergency_pause' ORDER BY created_at DESC LIMIT 5;
   ```
3. Determine root cause:
   - Was it triggered automatically by auto-pause rules?
   - Was it triggered manually by security team due to an exploit attempt?

## Mitigation & Recovery
1. **Security Review**:
   - Ensure funds and state are safe and no ongoing malicious exploit is present.
2. **Authorize Unpause**:
   - The contract admin or multisig threshold signers must submit `unpause()`:
     ```bash
     soroban contract invoke \
       --id <PAYMENT_ESCROW_CONTRACT_ID> \
       --network testnet \
       --source <ADMIN_ACCOUNT> \
       -- unpause
     ```
3. **Verify Restoration**:
   - Submit a test micro-escrow or read contract state to confirm `is_paused == false`.
   - Resume backend remittance processor worker.
