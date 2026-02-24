use soroban_sdk::{contracttype, contracterror, BytesN, Env, Address, symbol_short};

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
}

// ---------------------------------------------------------------------------
// Storage keys (isolated from business-logic keys)
// ---------------------------------------------------------------------------

#[derive(Clone, Copy)]
#[contracttype]
pub enum UpgradeDataKey {
    Version,
    Paused,
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
        .set(&UpgradeDataKey::Paused, &false);
}

/// Pause the contract. Admin-only.
pub fn pause(env: &Env, admin: &Address) -> Result<(), UpgradeError> {
    admin.require_auth();
    if is_paused(env) {
        return Err(UpgradeError::AlreadyPaused);
    }
    env.storage()
        .instance()
        .set(&UpgradeDataKey::Paused, &true);
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

/// Upgrade the contract WASM. Admin-only.
///
/// The contract is paused before the WASM is replaced and the version is
/// incremented.  Call [`migrate`] on the **new** code to finalize the
/// upgrade and unpause.
///
/// # Soroban upgrade model
///
/// `env.deployer().update_current_contract_wasm()` replaces the executable
/// for all **future** invocations while preserving the contract address and
/// all stored data — this is the Soroban-native equivalent of a proxy /
/// delegatecall pattern.
pub fn upgrade(
    env: &Env,
    admin: &Address,
    new_wasm_hash: BytesN<32>,
) -> Result<(), UpgradeError> {
    admin.require_auth();

    // Pause during upgrade
    env.storage()
        .instance()
        .set(&UpgradeDataKey::Paused, &true);

    // Bump version
    let current = get_version(env);
    let new_version = current + 1;
    env.storage()
        .instance()
        .set(&UpgradeDataKey::Version, &new_version);

    // Emit event before WASM swap
    env.events().publish(
        (symbol_short!("upgraded"),),
        (new_version, new_wasm_hash.clone()),
    );

    // Replace the contract code (takes effect from the next invocation)
    env.deployer()
        .update_current_contract_wasm(new_wasm_hash);

    Ok(())
}

/// Finalize a migration after an upgrade. Admin-only.
///
/// The **new** code should override this to perform any data-schema
/// transformations, then call this helper to unpause and emit the
/// migration event.
pub fn migrate(env: &Env, admin: &Address) -> Result<u32, UpgradeError> {
    admin.require_auth();

    // Unpause
    env.storage()
        .instance()
        .set(&UpgradeDataKey::Paused, &false);

    let version = get_version(env);
    env.events()
        .publish((symbol_short!("migrated"),), version);

    Ok(version)
}
