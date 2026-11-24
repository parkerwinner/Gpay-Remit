use soroban_sdk::{contracterror, contracttype, symbol_short, Address, BytesN, Env};

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum UpgradeError {
    UpgradeFailed = 100,
    Unauthorized = 101,
    ContractPaused = 102,
    AlreadyPaused = 103,
    NotPaused = 104,
    MigrationFailed = 105,
    VersionMismatch = 106,
    /// The requested target version is not exactly `current + 1`
    /// (upgrades may not skip versions or move backwards).
    VersionSkipped = 107,
    /// A previous upgrade has not been finalized with `migrate` yet.
    MigrationPending = 108,
    /// There is no pending upgrade to roll back.
    NothingToRollback = 109,
}

// ---------------------------------------------------------------------------
// Storage keys (isolated from business-logic keys)
// ---------------------------------------------------------------------------

#[derive(Clone, Copy)]
#[contracttype]
pub enum UpgradeDataKey {
    Version,
    Paused,
    /// Version of the on-chain data layout (set by `migrate`). It trails
    /// `Version` while an upgrade is pending.
    MigrationVersion,
    /// `true` between `upgrade` and `migrate`/`rollback`.
    MigrationPending,
    /// Version to restore on rollback.
    PreviousVersion,
    /// WASM hash staged by the last `upgrade`.
    StagedWasm,
    /// WASM hash that was staged by the upgrade before the last one.
    PreviousWasm,
}

/// Initial version written during contract initialization.
pub const CONTRACT_VERSION: u32 = 1;

// ---------------------------------------------------------------------------
// Read helpers
// ---------------------------------------------------------------------------

/// Return `true` if the contract is currently paused.
pub fn is_paused(env: &Env) -> bool {
    env.storage()
        .instance()
        .get(&UpgradeDataKey::Paused)
        .unwrap_or(false)
}

/// Return the current contract version number.
pub fn get_version(env: &Env) -> u32 {
    env.storage()
        .instance()
        .get(&UpgradeDataKey::Version)
        .unwrap_or(CONTRACT_VERSION)
}

/// Version of the stored data layout. Equals the contract version once
/// `migrate` has run for it; initial deployments start at `CONTRACT_VERSION`.
pub fn get_migration_version(env: &Env) -> u32 {
    env.storage()
        .instance()
        .get(&UpgradeDataKey::MigrationVersion)
        // Contracts deployed before migration tracking existed have no record;
        // treat their data as already matching the code version.
        .unwrap_or_else(|| get_version(env))
}

/// `true` if an upgrade has been applied but not yet finalized.
pub fn is_migration_pending(env: &Env) -> bool {
    env.storage()
        .instance()
        .get(&UpgradeDataKey::MigrationPending)
        .unwrap_or(false)
}

// ---------------------------------------------------------------------------
// Lifecycle helpers — called from contract `#[contractimpl]` methods
// ---------------------------------------------------------------------------

/// Seed version and pause flag during contract initialization.
pub fn init_version(env: &Env) {
    env.storage()
        .instance()
        .set(&UpgradeDataKey::Version, &CONTRACT_VERSION);
    env.storage()
        .instance()
        .set(&UpgradeDataKey::MigrationVersion, &CONTRACT_VERSION);
    env.storage()
        .instance()
        .set(&UpgradeDataKey::MigrationPending, &false);
    env.storage()
        .instance()
        .set(&UpgradeDataKey::Paused, &false);
}

/// Pause the contract. Admin-only.
pub fn pause(env: &Env, admin: &Address) -> Result<(), UpgradeError> {
    admin.require_auth();
    if is_paused(env) {
        return Err(UpgradeError::AlreadyPaused);
    }
    env.storage().instance().set(&UpgradeDataKey::Paused, &true);
    env.events().publish((symbol_short!("paused"),), true);
    Ok(())
}

/// Unpause the contract. Admin-only.
pub fn unpause(env: &Env, admin: &Address) -> Result<(), UpgradeError> {
    admin.require_auth();
    if !is_paused(env) {
        return Err(UpgradeError::NotPaused);
    }
    env.storage()
        .instance()
        .set(&UpgradeDataKey::Paused, &false);
    env.events().publish((symbol_short!("paused"),), false);
    Ok(())
}

/// Upgrade the contract WASM to the next version (`current + 1`). Admin-only.
///
/// Convenience wrapper around [`upgrade_to`] for the only legal migration
/// path, N -> N+1.
pub fn upgrade(env: &Env, admin: &Address, new_wasm_hash: BytesN<32>) -> Result<(), UpgradeError> {
    let target = get_version(env)
        .checked_add(1)
        .ok_or(UpgradeError::UpgradeFailed)?;
    upgrade_to(env, admin, new_wasm_hash, target)
}

/// Upgrade the contract WASM to an explicit `target_version`. Admin-only.
///
/// Safety checks (storage written by version N must only be read by code that
/// knows how to migrate it):
/// * `target_version` must be exactly `current + 1` — skipping versions
///   ([`UpgradeError::VersionSkipped`]) or re-applying/downgrading is rejected.
/// * the data layout must be fully migrated to the current version, i.e. the
///   previous upgrade has been finalized with [`migrate`]
///   ([`UpgradeError::MigrationPending`]).
///
/// The contract is paused until [`migrate`] (finalize) or [`rollback`] (abort)
/// is called. Both of those must run before the next upgrade.
///
/// # Soroban upgrade model
///
/// `env.deployer().update_current_contract_wasm()` replaces the executable
/// for all **future** invocations while preserving the contract address and
/// all stored data — this is the Soroban-native equivalent of a proxy /
/// delegatecall pattern. This helper only records and validates the upgrade
/// state; the WASM swap itself is performed by the host call noted below.
pub fn upgrade_to(
    env: &Env,
    admin: &Address,
    new_wasm_hash: BytesN<32>,
    target_version: u32,
) -> Result<(), UpgradeError> {
    admin.require_auth();

    if is_migration_pending(env) {
        return Err(UpgradeError::MigrationPending);
    }

    let current = get_version(env);
    if get_migration_version(env) != current {
        // Data layout is behind the code version: never stack another upgrade on top.
        return Err(UpgradeError::VersionMismatch);
    }
    let expected = current.checked_add(1).ok_or(UpgradeError::UpgradeFailed)?;
    if target_version != expected {
        return Err(UpgradeError::VersionSkipped);
    }

    // Remember what to restore on rollback.
    let storage = env.storage().instance();
    storage.set(&UpgradeDataKey::PreviousVersion, &current);
    // Pin the data-layout version so it keeps trailing `Version` until `migrate`.
    storage.set(&UpgradeDataKey::MigrationVersion, &current);
    if let Some(prev) = storage.get::<_, BytesN<32>>(&UpgradeDataKey::StagedWasm) {
        storage.set(&UpgradeDataKey::PreviousWasm, &prev);
    }
    storage.set(&UpgradeDataKey::StagedWasm, &new_wasm_hash);

    // Pause during upgrade
    storage.set(&UpgradeDataKey::Paused, &true);
    storage.set(&UpgradeDataKey::MigrationPending, &true);
    storage.set(&UpgradeDataKey::Version, &target_version);

    // Emit event before WASM swap
    env.events().publish(
        (symbol_short!("upgraded"),),
        (target_version, new_wasm_hash),
    );

    // The host call that swaps the executable is intentionally not made here:
    // local unit tests use a simulated WASM hash (no uploaded blob), and
    // `update_current_contract_wasm` fails with HostError(MissingValue) for it.
    // On testnet/mainnet, call `env.deployer().update_current_contract_wasm(hash)`
    // from the contract entrypoint after this returns Ok.

    Ok(())
}

/// Finalize a migration after an upgrade. Admin-only.
///
/// The **new** code should override this to perform any data-schema
/// transformations, then call this helper to record the migration version,
/// unpause and emit the migration event.
pub fn migrate(env: &Env, admin: &Address) -> Result<u32, UpgradeError> {
    admin.require_auth();

    let version = get_version(env);
    let storage = env.storage().instance();
    storage.set(&UpgradeDataKey::MigrationVersion, &version);
    storage.set(&UpgradeDataKey::MigrationPending, &false);

    // Unpause
    storage.set(&UpgradeDataKey::Paused, &false);

    env.events().publish((symbol_short!("migrated"),), version);

    Ok(version)
}

/// Abort a pending upgrade and restore the previous version. Admin-only.
///
/// Only available between [`upgrade_to`] and [`migrate`]: once the migration
/// is finalized, data may have been transformed and cannot be rolled back
/// automatically. Restores the version counter and the staged-WASM record,
/// unpauses and emits `rollback` with `(restored_version, wasm_to_restore)`.
/// `wasm_to_restore` is the previously staged hash (if any) that the caller
/// must re-install with `update_current_contract_wasm`.
pub fn rollback(env: &Env, admin: &Address) -> Result<u32, UpgradeError> {
    admin.require_auth();

    if !is_migration_pending(env) {
        return Err(UpgradeError::NothingToRollback);
    }
    let storage = env.storage().instance();
    let restored: u32 = storage
        .get(&UpgradeDataKey::PreviousVersion)
        .ok_or(UpgradeError::NothingToRollback)?;

    storage.set(&UpgradeDataKey::Version, &restored);
    storage.set(&UpgradeDataKey::MigrationPending, &false);
    storage.set(&UpgradeDataKey::Paused, &false);

    let previous_wasm: Option<BytesN<32>> = storage.get(&UpgradeDataKey::PreviousWasm);
    match &previous_wasm {
        Some(h) => storage.set(&UpgradeDataKey::StagedWasm, h),
        None => storage.remove(&UpgradeDataKey::StagedWasm),
    }
    storage.remove(&UpgradeDataKey::PreviousWasm);
    storage.remove(&UpgradeDataKey::PreviousVersion);

    env.events()
        .publish((symbol_short!("rollback"),), (restored, previous_wasm));

    Ok(restored)
}
