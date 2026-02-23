# Upgradeable Contract Pattern

Gpay-Remit smart contracts implement Soroban's native upgrade pattern,
allowing seamless logic updates without data loss or downtime.

## How It Works

Soroban contracts are identified by their **address**, not their WASM code.
When `env.deployer().update_current_contract_wasm(new_hash)` is called:

1. The contract address stays the same.
2. All stored data (escrows, invoices, KYC records) is preserved.
3. Future invocations execute the **new** WASM code.

This is the Soroban-native equivalent of a proxy / delegatecall pattern.

## Upgrade Flow

```
Admin                Contract (v1)             Contract (v2)
  |                       |                         |
  |-- upgrade(hash) ----->|                         |
  |   [pauses contract]   |                         |
  |   [bumps version]     |                         |
  |   [emits "upgraded"]  |                         |
  |   [swaps WASM]        |                         |
  |                       |                         |
  |-- migrate() --------->|------------------------>|
  |                       |  [runs new code]        |
  |                       |  [data migration]       |
  |                       |  [unpauses]             |
  |                       |  [emits "migrated"]     |
```

### Step 1 — Upgrade

```
PaymentEscrowContract::upgrade(admin, new_wasm_hash)
RemittanceHubContract::upgrade(admin, new_wasm_hash)
```

- Requires admin auth.
- Pauses the contract (all state-changing operations blocked).
- Increments the version counter.
- Emits an `upgraded` event with `(new_version, wasm_hash)`.
- Replaces the WASM (takes effect from the next invocation).

### Step 2 — Migrate

```
PaymentEscrowContract::migrate(admin)
RemittanceHubContract::migrate(admin)
```

- Called on the **new** contract code.
- Perform any data-schema transformations here (override in new code).
- Unpauses the contract.
- Emits a `migrated` event with the version number.
- Returns the new version.

## Pause Mechanism

Contracts can be paused independently of upgrades:

```
pause(admin)    -- blocks critical operations
unpause(admin)  -- resumes normal operation
is_paused()     -- read-only status check
```

While paused, the following operations return a `ContractPaused` error:

| PaymentEscrowContract | RemittanceHubContract |
|---|---|
| `create_escrow` | `send_remittance` |
| `deposit` | `complete_remittance` |
| `release_escrow` | `generate_invoice` |
| `release_partial` | `mark_invoice_paid` |
| `refund_escrow` | |
| `refund_partial` | |

Read-only getters and admin configuration calls remain available.

## Versioning

```
version() -> u32
```

Starts at `1` after initialization and increments by 1 on each upgrade.
The version is stored in instance storage under `UpgradeDataKey::Version`.

## Storage Isolation

Upgrade-related keys live in a separate `UpgradeDataKey` enum
(`Version`, `Paused`) to avoid collisions with business-logic storage.

## Error Types

| Error | Code | Description |
|---|---|---|
| `UpgradeFailed` | 100 | WASM replacement failed |
| `Unauthorized` | 101 | Caller is not the contract admin |
| `ContractPaused` | 102 | Operation blocked while paused |
| `AlreadyPaused` | 103 | Contract is already paused |
| `NotPaused` | 104 | Contract is not currently paused |
| `MigrationFailed` | 105 | Data migration error |
| `VersionMismatch` | 106 | Expected version does not match |

## Events

| Event | Data | Emitted by |
|---|---|---|
| `upgraded` | `(version: u32, wasm_hash: BytesN<32>)` | `upgrade()` |
| `migrated` | `version: u32` | `migrate()` |
| `paused` | `true` | `pause()` |
| `paused` | `false` | `unpause()` |

## Security Considerations

- Only the stored admin can call `upgrade`, `migrate`, `pause`, `unpause`.
- The contract is automatically paused during upgrades to prevent
  state mutations against stale logic.
- Multi-sig admin support can be layered on top via a governance contract
  that holds the admin role.
- All upgrade and pause events are emitted for off-chain monitoring.

## Testing

Unit tests cover:

- Version tracking after initialization.
- Pause / unpause lifecycle.
- Paused contract blocks `create_escrow` (and other guarded operations).
- Non-admin callers are rejected.

Integration testing of `update_current_contract_wasm` should be performed
on Stellar Testnet, as the Soroban test environment may not fully replicate
WASM hot-swapping behaviour.

## Deploying an Upgrade on Testnet

```bash
# 1. Build the new contract WASM
soroban contract build

# 2. Upload the new WASM and get its hash
soroban contract upload \
  --wasm target/wasm32-unknown-unknown/release/gpay_remit_contracts.wasm \
  --network testnet --source admin

# 3. Call upgrade on the existing contract
soroban contract invoke --id <CONTRACT_ID> \
  --network testnet --source admin \
  -- upgrade --admin <ADMIN_ADDRESS> --new_wasm_hash <HASH>

# 4. Call migrate to finalize and unpause
soroban contract invoke --id <CONTRACT_ID> \
  --network testnet --source admin \
  -- migrate --admin <ADMIN_ADDRESS>

# 5. Verify
soroban contract invoke --id <CONTRACT_ID> \
  --network testnet -- version
```
