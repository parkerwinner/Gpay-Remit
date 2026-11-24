use soroban_sdk::{contracterror, contracttype, symbol_short, Address, Env, Symbol, Vec};

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum PauseError {
    /// Caller is not authorized.
    Unauthorized = 50,
    /// Signer has already voted on this action.
    AlreadyVoted = 51,
    /// Quorum has not been reached yet.
    QuorumNotReached = 52,
    /// No active pause vote in progress.
    NoActiveVote = 53,
    /// Pause vote has expired.
    VoteExpired = 54,
    /// Signer is not registered.
    NotASigner = 55,
    /// Contract is already paused.
    AlreadyPaused = 56,
    /// Contract is not paused.
    NotPaused = 57,
    /// Minimum signers not met.
    InsufficientSigners = 58,
    /// Auto-unpause duration must be greater than zero.
    InvalidDuration = 59,
    /// Requested pause expiration is not later than the current expiration
    /// (or the current ledger), so the extension would not extend anything.
    InvalidExpiration = 60,
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum PauseAction {
    Pause,
    Unpause,
}

#[derive(Clone)]
#[contracttype]
pub struct PauseConfig {
    /// Admin who initialized the pause mechanism.
    pub admin: Address,
    /// List of authorized pause signers.
    pub signers: Vec<Address>,
    /// Number of signatures required to pause/unpause (quorum).
    pub quorum: u32,
    /// Timeout in seconds for a pause vote to remain valid.
    pub vote_timeout: u64,
    /// Number of ledgers an activated pause lasts before it automatically
    /// expires. Guarantees a pause cannot outlive a lost admin indefinitely.
    pub pause_duration: u32,
    /// Ledger sequence at which the active pause expires. `Some` while the
    /// contract is paused; `None` when it is not paused.
    pub pause_expiration: Option<u32>,
}

#[derive(Clone)]
#[contracttype]
pub struct PauseVote {
    /// The action being voted on.
    pub action: PauseAction,
    /// List of signers who have voted.
    pub voters: Vec<Address>,
    /// Number of votes received.
    pub vote_count: u32,
    /// Timestamp when the vote was created.
    pub created_at: u64,
}

// ---------------------------------------------------------------------------
// Storage Keys
// ---------------------------------------------------------------------------

#[derive(Clone, Copy)]
#[contracttype]
pub enum PauseStorageKey {
    Config,
    ActiveVote,
    PauseState,
}

// ---------------------------------------------------------------------------
// Public functions
// ---------------------------------------------------------------------------

/// Initialize the multi-sig pause mechanism. Admin-only.
///
/// `pause_duration` is the number of ledgers a pause lasts once quorum is
/// reached; after that the contract unpauses automatically (see
/// [`is_paused`]). It must be greater than zero.
pub fn init_pause_config(
    env: &Env,
    admin: &Address,
    signers: &Vec<Address>,
    quorum: u32,
    vote_timeout: u64,
    pause_duration: u32,
) -> Result<(), PauseError> {
    admin.require_auth();

    if signers.len() < 2 {
        return Err(PauseError::InsufficientSigners);
    }

    if quorum == 0 || quorum > signers.len() {
        return Err(PauseError::QuorumNotReached);
    }

    if pause_duration == 0 {
        return Err(PauseError::InvalidDuration);
    }

    let config = PauseConfig {
        admin: admin.clone(),
        signers: signers.clone(),
        quorum,
        vote_timeout,
        pause_duration,
        pause_expiration: None,
    };

    env.storage()
        .persistent()
        .set(&PauseStorageKey::Config, &config);
    env.storage()
        .persistent()
        .set(&PauseStorageKey::PauseState, &false);

    env.events().publish(
        (symbol_short!("pause"), symbol_short!("init")),
        (admin.clone(), quorum, vote_timeout),
    );

    Ok(())
}

/// Add a new signer to the pause mechanism. Admin-only.
pub fn add_signer(env: &Env, admin: &Address, signer: &Address) -> Result<(), PauseError> {
    admin.require_auth();

    let mut config: PauseConfig = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::Config)
        .ok_or(PauseError::Unauthorized)?;

    if *admin != config.admin {
        return Err(PauseError::Unauthorized);
    }

    config.signers.push_back(signer.clone());
    env.storage()
        .persistent()
        .set(&PauseStorageKey::Config, &config);

    env.events().publish(
        (symbol_short!("pause"), symbol_short!("sig_add")),
        signer.clone(),
    );

    Ok(())
}

/// Remove a signer from the pause mechanism. Admin-only.
pub fn remove_signer(env: &Env, admin: &Address, signer: &Address) -> Result<(), PauseError> {
    admin.require_auth();

    let mut config: PauseConfig = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::Config)
        .ok_or(PauseError::Unauthorized)?;

    if *admin != config.admin {
        return Err(PauseError::Unauthorized);
    }

    let mut new_signers: Vec<Address> = Vec::new(env);
    for s in config.signers.iter() {
        if s != *signer {
            new_signers.push_back(s);
        }
    }

    if new_signers.len() < 2 {
        return Err(PauseError::InsufficientSigners);
    }

    if config.quorum > new_signers.len() {
        config.quorum = new_signers.len();
    }

    config.signers = new_signers;
    env.storage()
        .persistent()
        .set(&PauseStorageKey::Config, &config);

    env.events().publish(
        (symbol_short!("pause"), symbol_short!("signer_rm")),
        signer.clone(),
    );

    Ok(())
}

/// Vote to pause the contract. Only registered signers can vote.
pub fn vote_pause(env: &Env, signer: &Address) -> Result<(), PauseError> {
    signer.require_auth();
    expire_pause_if_due(env);

    let mut config: PauseConfig = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::Config)
        .ok_or(PauseError::Unauthorized)?;

    if !config.signers.contains(signer) {
        return Err(PauseError::NotASigner);
    }

    let is_paused: bool = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::PauseState)
        .unwrap_or(false);

    if is_paused {
        return Err(PauseError::AlreadyPaused);
    }

    let now = env.ledger().timestamp();

    let mut vote: PauseVote = match env.storage().persistent().get(&PauseStorageKey::ActiveVote) {
        Some(v) => v,
        None => PauseVote {
            action: PauseAction::Pause,
            voters: Vec::new(env),
            vote_count: 0,
            created_at: now,
        },
    };

    // Check if vote has expired
    if now > vote.created_at + config.vote_timeout {
        // Reset the vote
        vote = PauseVote {
            action: PauseAction::Pause,
            voters: Vec::new(env),
            vote_count: 0,
            created_at: now,
        };
    }

    if vote.voters.contains(signer) {
        return Err(PauseError::AlreadyVoted);
    }

    vote.voters.push_back(signer.clone());
    vote.vote_count += 1;

    env.events().publish(
        (symbol_short!("pause"), symbol_short!("vote")),
        (signer.clone(), vote.vote_count, config.quorum),
    );

    // Check if quorum is reached
    if vote.vote_count >= config.quorum {
        let expiration = env
            .ledger()
            .sequence()
            .saturating_add(config.pause_duration);
        config.pause_expiration = Some(expiration);
        env.storage()
            .persistent()
            .set(&PauseStorageKey::Config, &config);
        env.storage()
            .persistent()
            .set(&PauseStorageKey::PauseState, &true);
        env.storage()
            .persistent()
            .remove(&PauseStorageKey::ActiveVote);

        env.events().publish(
            (symbol_short!("pause"), symbol_short!("paused")),
            (vote.vote_count, expiration),
        );
    } else {
        env.storage()
            .persistent()
            .set(&PauseStorageKey::ActiveVote, &vote);
    }

    Ok(())
}

/// Vote to unpause the contract. Only registered signers can vote.
pub fn vote_unpause(env: &Env, signer: &Address) -> Result<(), PauseError> {
    signer.require_auth();
    expire_pause_if_due(env);

    let mut config: PauseConfig = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::Config)
        .ok_or(PauseError::Unauthorized)?;

    if !config.signers.contains(signer) {
        return Err(PauseError::NotASigner);
    }

    let is_paused: bool = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::PauseState)
        .unwrap_or(false);

    if !is_paused {
        return Err(PauseError::NotPaused);
    }

    let now = env.ledger().timestamp();

    let mut vote: PauseVote = match env.storage().persistent().get(&PauseStorageKey::ActiveVote) {
        Some(v) => v,
        None => PauseVote {
            action: PauseAction::Unpause,
            voters: Vec::new(env),
            vote_count: 0,
            created_at: now,
        },
    };

    // Check if vote has expired
    if now > vote.created_at + config.vote_timeout {
        vote = PauseVote {
            action: PauseAction::Unpause,
            voters: Vec::new(env),
            vote_count: 0,
            created_at: now,
        };
    }

    if vote.voters.contains(signer) {
        return Err(PauseError::AlreadyVoted);
    }

    vote.voters.push_back(signer.clone());
    vote.vote_count += 1;

    env.events().publish(
        (symbol_short!("pause"), symbol_short!("vote")),
        (signer.clone(), vote.vote_count, config.quorum),
    );

    if vote.vote_count >= config.quorum {
        config.pause_expiration = None;
        env.storage()
            .persistent()
            .set(&PauseStorageKey::Config, &config);
        env.storage()
            .persistent()
            .set(&PauseStorageKey::PauseState, &false);
        env.storage()
            .persistent()
            .remove(&PauseStorageKey::ActiveVote);

        env.events().publish(
            (symbol_short!("pause"), symbol_short!("unpaused")),
            vote.vote_count,
        );
    } else {
        env.storage()
            .persistent()
            .set(&PauseStorageKey::ActiveVote, &vote);
    }

    Ok(())
}

/// If the active pause has outlived its expiration ledger, unpause the
/// contract and emit `PauseExpired`.
///
/// Soroban has no scheduler, so expiry is applied lazily: every entry point
/// that depends on the pause state calls this first. A pause is in effect
/// while the current ledger sequence is `<= pause_expiration`.
fn expire_pause_if_due(env: &Env) {
    let paused: bool = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::PauseState)
        .unwrap_or(false);
    if !paused {
        return;
    }

    let mut config: PauseConfig = match env.storage().persistent().get(&PauseStorageKey::Config) {
        Some(c) => c,
        None => return,
    };
    let expiration = match config.pause_expiration {
        Some(e) => e,
        None => return,
    };
    if env.ledger().sequence() <= expiration {
        return;
    }

    config.pause_expiration = None;
    env.storage()
        .persistent()
        .set(&PauseStorageKey::Config, &config);
    env.storage()
        .persistent()
        .set(&PauseStorageKey::PauseState, &false);

    env.events().publish(
        (symbol_short!("pause"), symbol_short!("expired")),
        expiration,
    );
}

/// Check if the contract is paused via the multi-sig mechanism.
///
/// Applies automatic expiry first, so this returns `false` once the pause
/// duration has elapsed even if nobody has called an unpause vote.
pub fn is_paused(env: &Env) -> bool {
    expire_pause_if_due(env);
    env.storage()
        .persistent()
        .get(&PauseStorageKey::PauseState)
        .unwrap_or(false)
}

/// Extend the active pause to `new_expiration` (a ledger sequence). Admin-only.
///
/// The new expiration must be later than the current ledger and later than
/// the existing expiration, so this can only lengthen a pause.
pub fn extend_pause(env: &Env, admin: &Address, new_expiration: u32) -> Result<(), PauseError> {
    admin.require_auth();
    expire_pause_if_due(env);

    let mut config: PauseConfig = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::Config)
        .ok_or(PauseError::Unauthorized)?;

    if *admin != config.admin {
        return Err(PauseError::Unauthorized);
    }

    let paused: bool = env
        .storage()
        .persistent()
        .get(&PauseStorageKey::PauseState)
        .unwrap_or(false);
    if !paused {
        return Err(PauseError::NotPaused);
    }

    if new_expiration <= env.ledger().sequence() {
        return Err(PauseError::InvalidExpiration);
    }
    if let Some(current) = config.pause_expiration {
        if new_expiration <= current {
            return Err(PauseError::InvalidExpiration);
        }
    }

    config.pause_expiration = Some(new_expiration);
    env.storage()
        .persistent()
        .set(&PauseStorageKey::Config, &config);

    env.events().publish(
        (symbol_short!("pause"), symbol_short!("extended")),
        new_expiration,
    );

    Ok(())
}

/// Get the current pause configuration.
pub fn get_config(env: &Env) -> Option<PauseConfig> {
    env.storage().persistent().get(&PauseStorageKey::Config)
}

/// Get the current active pause vote, if any.
pub fn get_active_vote(env: &Env) -> Option<PauseVote> {
    env.storage().persistent().get(&PauseStorageKey::ActiveVote)
}

/// Get the current pause state.
pub fn get_pause_state(env: &Env) -> bool {
    is_paused(env)
}

#[cfg(test)]
mod tests {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger as _};
    use soroban_sdk::{contract, contractimpl};

    /// Empty contract whose address scopes the pause storage in tests.
    #[contract]
    pub struct PauseTestContract;

    #[contractimpl]
    impl PauseTestContract {}

    const START_LEDGER: u32 = 1_000;
    const PAUSE_DURATION: u32 = 100;

    fn make_signers(env: &Env) -> Vec<Address> {
        let mut signers = Vec::new(env);
        signers.push_back(Address::generate(env));
        signers.push_back(Address::generate(env));
        signers.push_back(Address::generate(env));
        signers
    }

    /// Returns (env, contract address, admin, signers) with the pause
    /// mechanism initialized at `START_LEDGER`.
    fn setup() -> (Env, Address, Address, Vec<Address>) {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().set_sequence_number(START_LEDGER);
        let contract_id = env.register_contract(None, PauseTestContract);
        let admin = Address::generate(&env);
        let signers = make_signers(&env);
        env.as_contract(&contract_id, || {
            init_pause_config(&env, &admin, &signers, 2, 1_000, PAUSE_DURATION).unwrap();
        });
        (env, contract_id, admin, signers)
    }

    /// Reach quorum (2 of 3) to pause the contract.
    fn pause_with_quorum(env: &Env, signers: &Vec<Address>) {
        vote_pause(env, &signers.get(0).unwrap()).unwrap();
        vote_pause(env, &signers.get(1).unwrap()).unwrap();
    }

    #[test]
    fn init_rejects_zero_pause_duration() {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register_contract(None, PauseTestContract);
        let admin = Address::generate(&env);
        let signers = make_signers(&env);
        env.as_contract(&contract_id, || {
            assert_eq!(
                init_pause_config(&env, &admin, &signers, 2, 1_000, 0),
                Err(PauseError::InvalidDuration)
            );
        });
    }

    #[test]
    fn quorum_pause_records_expiration_ledger() {
        let (env, contract_id, _admin, signers) = setup();
        env.as_contract(&contract_id, || {
            pause_with_quorum(&env, &signers);
            assert!(is_paused(&env));
            let config = get_config(&env).unwrap();
            assert_eq!(config.pause_expiration, Some(START_LEDGER + PAUSE_DURATION));
        });
    }

    #[test]
    fn pause_auto_unpauses_after_configured_period() {
        let (env, contract_id, _admin, signers) = setup();
        env.as_contract(&contract_id, || {
            pause_with_quorum(&env, &signers);
            let expiration = START_LEDGER + PAUSE_DURATION;

            // Still paused on the expiration ledger itself.
            env.ledger().set_sequence_number(expiration);
            assert!(is_paused(&env));

            // Auto-unpaused one ledger later, with no unpause vote cast.
            env.ledger().set_sequence_number(expiration + 1);
            assert!(!is_paused(&env));
            assert!(!get_pause_state(&env));
            assert_eq!(get_config(&env).unwrap().pause_expiration, None);
        });
    }

    #[test]
    fn expired_pause_does_not_block_a_new_pause_vote() {
        let (env, contract_id, _admin, signers) = setup();
        env.as_contract(&contract_id, || {
            pause_with_quorum(&env, &signers);
            env.ledger()
                .set_sequence_number(START_LEDGER + PAUSE_DURATION + 1);

            // After expiry the contract is unpaused, so a fresh pause is allowed.
            pause_with_quorum(&env, &signers);
            assert!(is_paused(&env));
        });
    }

    #[test]
    fn admin_can_extend_active_pause() {
        let (env, contract_id, admin, signers) = setup();
        env.as_contract(&contract_id, || {
            pause_with_quorum(&env, &signers);
            let extended = START_LEDGER + PAUSE_DURATION * 3;
            extend_pause(&env, &admin, extended).unwrap();
            assert_eq!(get_config(&env).unwrap().pause_expiration, Some(extended));

            // The original expiration no longer ends the pause.
            env.ledger()
                .set_sequence_number(START_LEDGER + PAUSE_DURATION + 1);
            assert!(is_paused(&env));

            env.ledger().set_sequence_number(extended + 1);
            assert!(!is_paused(&env));
        });
    }

    #[test]
    fn extend_pause_rejects_non_admin_and_non_extending_values() {
        let (env, contract_id, _admin, signers) = setup();
        env.as_contract(&contract_id, || {
            let admin = get_config(&env).unwrap().admin;
            let stranger = Address::generate(&env);
            assert_eq!(
                extend_pause(&env, &admin, START_LEDGER + 500),
                Err(PauseError::NotPaused),
                "extension is only meaningful while paused"
            );

            pause_with_quorum(&env, &signers);
            assert_eq!(
                extend_pause(&env, &stranger, START_LEDGER + 500),
                Err(PauseError::Unauthorized)
            );

            let current = START_LEDGER + PAUSE_DURATION;
            assert_eq!(
                extend_pause(&env, &admin, current),
                Err(PauseError::InvalidExpiration),
                "must be later than the current expiration"
            );
        });
    }
}
