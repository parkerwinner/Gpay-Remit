use soroban_sdk::{contracttype, Address, Env, symbol_short};

/// Rate limiting module for Gpay-Remit contracts.
///
/// Provides per-user and global rate limiting using ledger timestamps
/// with sliding window tracking. Admin-configurable limits and
/// exemptions for trusted addresses.

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum FunctionType {
    Deposit,
    Release,
    Refund,
    Remittance,
    Invoice,
}

#[derive(Clone, Debug)]
#[contracttype]
pub struct RateLimitConfig {
    pub enabled: bool,
    pub max_count: u32,
    pub interval: u64,
}

#[derive(Clone, Debug)]
#[contracttype]
pub struct RateLimitEntry {
    pub last_call_time: u64,
    pub count: u32,
    pub window_start: u64,
}

#[derive(Clone)]
#[contracttype]
pub enum RateLimitKey {
    Config,
    GlobalConfig,
    Exempt(Address),
    UserLimit(Address, FunctionType),
    GlobalCount(FunctionType),
}

/// Check and enforce rate limit for a caller + function type.
///
/// Returns `true` if the call is allowed, `false` if rate limited.
/// Automatically updates counters and resets windows on expiry.
/// Uses ledger timestamp for time intervals.
pub fn check_rate_limit(
    env: &Env,
    caller: &Address,
    function_type: FunctionType,
    admin: &Address,
) -> bool {
    // Check per-user config
    let config: Option<RateLimitConfig> =
        env.storage().instance().get(&RateLimitKey::Config);

    let config = match config {
        Some(c) => c,
        None => return true, // No config set, allow all
    };

    if !config.enabled {
        return true;
    }

    // Exempt: admin is always exempt
    if caller == admin {
        return true;
    }

    // Exempt: check whitelisted addresses
    let exempt: bool = env
        .storage()
        .instance()
        .get(&RateLimitKey::Exempt(caller.clone()))
        .unwrap_or(false);

    if exempt {
        return true;
    }

    let now = env.ledger().timestamp();

    // --- Per-user check ---
    let user_key = RateLimitKey::UserLimit(caller.clone(), function_type);
    let entry: Option<RateLimitEntry> = env.storage().temporary().get(&user_key);

    match entry {
        Some(mut e) => {
            if now.saturating_sub(e.window_start) >= config.interval {
                // Window expired, reset
                e.count = 1;
                e.window_start = now;
                e.last_call_time = now;
                env.storage().temporary().set(&user_key, &e);
            } else if e.count >= config.max_count {
                // Rate limit exceeded
                env.events().publish(
                    (symbol_short!("rl_hit"),),
                    (caller.clone(), function_type, e.count),
                );
                return false;
            } else {
                e.count += 1;
                e.last_call_time = now;
                env.storage().temporary().set(&user_key, &e);
            }
        }
        None => {
            let e = RateLimitEntry {
                last_call_time: now,
                count: 1,
                window_start: now,
            };
            env.storage().temporary().set(&user_key, &e);
        }
    }

    // --- Global limit check ---
    let global_config: Option<RateLimitConfig> =
        env.storage().instance().get(&RateLimitKey::GlobalConfig);

    if let Some(gc) = global_config {
        if gc.enabled {
            let global_key = RateLimitKey::GlobalCount(function_type);
            let global_entry: Option<RateLimitEntry> =
                env.storage().temporary().get(&global_key);

            match global_entry {
                Some(mut ge) => {
                    if now.saturating_sub(ge.window_start) >= gc.interval {
                        ge.count = 1;
                        ge.window_start = now;
                        ge.last_call_time = now;
                        env.storage().temporary().set(&global_key, &ge);
                    } else if ge.count >= gc.max_count {
                        env.events().publish(
                            (symbol_short!("rl_glob"),),
                            (function_type, ge.count),
                        );
                        return false;
                    } else {
                        ge.count += 1;
                        ge.last_call_time = now;
                        env.storage().temporary().set(&global_key, &ge);
                    }
                }
                None => {
                    let ge = RateLimitEntry {
                        last_call_time: now,
                        count: 1,
                        window_start: now,
                    };
                    env.storage().temporary().set(&global_key, &ge);
                }
            }
        }
    }

    true
}

/// Set per-user rate limit configuration.
pub fn set_config(env: &Env, config: RateLimitConfig) {
    env.storage().instance().set(&RateLimitKey::Config, &config);
}

/// Get per-user rate limit configuration.
pub fn get_config(env: &Env) -> Option<RateLimitConfig> {
    env.storage().instance().get(&RateLimitKey::Config)
}

/// Set global (platform-wide) rate limit configuration.
pub fn set_global_config(env: &Env, config: RateLimitConfig) {
    env.storage()
        .instance()
        .set(&RateLimitKey::GlobalConfig, &config);
}

/// Get global rate limit configuration.
pub fn get_global_config(env: &Env) -> Option<RateLimitConfig> {
    env.storage().instance().get(&RateLimitKey::GlobalConfig)
}

/// Set or remove rate limit exemption for an address.
pub fn set_exemption(env: &Env, address: &Address, exempt: bool) {
    env.storage()
        .instance()
        .set(&RateLimitKey::Exempt(address.clone()), &exempt);
}

/// Check if an address is exempt from rate limiting.
pub fn is_exempt(env: &Env, address: &Address) -> bool {
    env.storage()
        .instance()
        .get(&RateLimitKey::Exempt(address.clone()))
        .unwrap_or(false)
}
