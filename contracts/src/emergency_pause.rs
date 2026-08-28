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
pub fn init_pause_config(
    env: &Env,
    admin: &Address,
    signers: &Vec<Address>,
    quorum: u32,
    vote_timeout: u64,
) -> Result<(), PauseError> {
    admin.require_auth();

    if signers.len() < 2 {
        return Err(PauseError::InsufficientSigners);
    }

    if quorum == 0 || quorum > signers.len() {
        return Err(PauseError::QuorumNotReached);
    }

    let config = PauseConfig {
        admin: admin.clone(),
        signers: signers.clone(),
        quorum,
        vote_timeout,
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
pub fn add_signer(
    env: &Env,
    admin: &Address,
    signer: &Address,
) -> Result<(), PauseError> {
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
        (symbol_short!("pause"), symbol_short!("signer_add")),
        signer.clone(),
    );

    Ok(())
}

/// Remove a signer from the pause mechanism. Admin-only.
pub fn remove_signer(
    env: &Env,
    admin: &Address,
    signer: &Address,
) -> Result<(), PauseError> {
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

    let config: PauseConfig = env
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

    let mut vote: PauseVote = match env
        .storage()
        .persistent()
        .get(&PauseStorageKey::ActiveVote)
    {
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
        env.storage()
            .persistent()
            .set(&PauseStorageKey::PauseState, &true);
        env.storage()
            .persistent()
            .remove(&PauseStorageKey::ActiveVote);

        env.events().publish(
            (symbol_short!("pause"), symbol_short!("paused")),
            vote.vote_count,
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

    let config: PauseConfig = env
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

    let mut vote: PauseVote = match env
        .storage()
        .persistent()
        .get(&PauseStorageKey::ActiveVote)
    {
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

/// Check if the contract is paused via the multi-sig mechanism.
pub fn is_paused(env: &Env) -> bool {
    env.storage()
        .persistent()
        .get(&PauseStorageKey::PauseState)
        .unwrap_or(false)
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
