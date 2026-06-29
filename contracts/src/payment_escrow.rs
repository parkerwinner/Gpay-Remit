use crate::events::{self, AssetRef, EventData};
use crate::kyc::{self, KycConfig, KycDataKey, KycRecord, KycStatus};
use crate::rate_limit::{self, FunctionType};
use crate::upgradeable;

use soroban_sdk::{
    contract, contracterror, contractimpl, contracttype, symbol_short, token, Address, BytesN, Env,
    Map, String, Vec,
};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum Error {
    /// Amount must be greater than zero.
    InvalidAmount = 1,
    /// Sender and recipient must be different addresses.
    SameSenderRecipient = 2,
    /// Escrow counter overflowed `u64`.
    CounterOverflow = 3,
    /// The requested escrow id does not exist.
    EscrowNotFound = 4,
    /// Operation is not valid for the escrow’s current status.
    InvalidStatus = 5,
    /// Escrow must be approved before this action.
    NotApproved = 6,
    /// Escrow is expired (current timestamp is past expiration).
    Expired = 7,
    /// Caller is not authorized to perform this action.
    Unauthorized = 8,
    /// Escrow has already been released.
    AlreadyReleased = 9,
    /// Escrow is not yet expired.
    NotExpired = 10,
    /// Caller is not the escrow sender.
    WrongSender = 11,
    /// Escrow must be pending/fundable for this action.
    EscrowNotPending = 12,
    /// Asset is not supported or invalid.
    InvalidAsset = 13,
    /// Provided amount is insufficient for the requested operation.
    InsufficientAmount = 14,
    /// Escrow is already funded.
    AlreadyFunded = 15,
    /// Deposited amount overflowed `i128`.
    DepositOverflow = 16,
    /// Release/refund conditions are not satisfied.
    ConditionsNotMet = 17,
    /// Caller is not allowed for this method.
    UnauthorizedCaller = 18,
    /// Not enough funds are available in escrow.
    InsufficientFunds = 19,
    /// Currency conversion failed.
    ConversionFailed = 20,
    /// Fee percentage must be within bounds (0–10000 bps).
    InvalidFeePercentage = 21,
    /// Partial release is disabled for this escrow.
    PartialReleaseNotAllowed = 22,
    /// Arithmetic overflow/underflow occurred.
    ArithmeticOverflow = 23,
    /// Escrow has already been refunded.
    AlreadyRefunded = 24,
    /// Refund is not authorized for the caller.
    UnauthorizedRefund = 25,
    /// No remaining funds are available for release/refund.
    NoFundsAvailable = 26,
    /// Refund amount is invalid (<= 0 or exceeds available).
    InvalidRefundAmount = 27,
    /// Provided signature does not match expected signer.
    SignatureMismatch = 28,
    /// Oracle call or validation failed.
    OracleFailure = 29,
    /// Current timestamp has not reached the required time.
    TimestampNotReached = 30,
    /// Approval is required before continuing.
    ApprovalRequired = 31,
    /// Total fees exceed or equal the escrow amount.
    FeeExceedsAmount = 32,
    /// Approval already exists for this escrow/approver.
    AlreadyApproved = 33,
    /// Multi-party quorum has not been met.
    QuorumNotMet = 34,
    /// Approver is not whitelisted for multi-party approval.
    ApproverNotWhitelisted = 35,
    /// Approval window has expired.
    ApprovalExpired = 36,
    /// Escrow is finalized and cannot be modified.
    EscrowFinalized = 37,
    /// Approval record was not found.
    ApprovalNotFound = 38,
    /// KYC checks failed for one or more parties.
    KycFailed = 39,
    /// KYC has not been configured.
    KycNotConfigured = 40,
    /// KYC proof/signature is required but missing.
    KycProofRequired = 41,
    /// Dispute already exists for this escrow.
    AlreadyDisputed = 42,
    /// Caller is not an arbitrator for this dispute.
    NotArbitrator = 43,
    /// Dispute record was not found.
    DisputeNotFound = 44,
    /// Voter has already voted on this dispute.
    AlreadyVoted = 45,
    /// Contract is paused (upgradeable pause flag set).
    ContractPaused = 46,
    /// Rate limit exceeded for this caller/function.
    RateLimitExceeded = 47,
    /// Escrow is non-compliant with registered rules.
    NonCompliant = 48,
}

#[derive(Clone, PartialEq, Eq, Debug)]
#[contracttype]
pub enum EscrowStatus {
    Pending,
    Funded,
    Approved,
    Released,
    Refunded,
    Expired,
    Disputed,
    Cancelled,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum RefundReason {
    Expiration,
    Dispute,
    UnmetConditions,
    SenderRequest,
    AdminAction,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum ConditionType {
    Timestamp,
    Approval,
    OraclePrice,
    MultiSignature,
    KYCVerified,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum ConditionOperator {
    And,
    Or,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum ComplianceRuleType {
    AmountThreshold,
    Jurisdiction,
    RegulatoryRequirement,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum ComplianceAction {
    Flag,
    Block,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct ComplianceRule {
    pub rule_type: ComplianceRuleType,
    pub threshold: i128,
    pub action: ComplianceAction,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct CancellationConfig {
    pub penalty_percentage: i128,
    pub recipient_compensation: i128,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum FeeType {
    Platform,
    Forex,
    Compliance,
    Network,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct FeeBreakdown {
    pub platform_fee: i128,
    pub forex_fee: i128,
    pub compliance_fee: i128,
    pub network_fee: i128,
    pub total_fee: i128,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct FeeStructure {
    pub platform_percentage: i128,
    pub forex_percentage: i128,
    pub compliance_flat: i128,
    pub network_flat: i128,
    pub min_fee: i128,
    pub max_fee: i128,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct Milestone {
    pub description: String,
    pub amount: i128,
    pub completed: bool,
    pub approved: bool,
    pub completed_by: Option<Address>,
    pub approved_by: Option<Address>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct InsuranceConfig {
    pub premium_rate: i128,
    pub coverage_limit: i128,
    pub insurer: Address,
    pub enabled: bool,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct EscrowInsurance {
    pub insured: bool,
    pub premium: i128,
    pub coverage: i128,
    pub claimed: bool,
    pub claim_reason: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct DelegationPermissions {
    pub can_release: bool,
    pub can_refund: bool,
    pub can_approve: bool,
    pub can_dispute: bool,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct DelegationEntry {
    pub delegate: Address,
    pub permissions: DelegationPermissions,
    pub delegated_at: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct EscrowAnalytics {
    pub total_volume: i128,
    pub total_escrows: u64,
    pub completed_escrows: u64,
    pub refunded_escrows: u64,
    pub disputed_escrows: u64,
    pub average_amount: i128,
    pub success_rate: i128,
    pub last_updated: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct Condition {
    pub condition_type: ConditionType,
    pub required: bool,
    pub verified: bool,
    pub threshold_value: i128,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct VerificationResult {
    pub all_passed: bool,
    pub failed_conditions: Vec<ConditionType>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct Asset {
    pub code: String,
    pub issuer: Address,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct ReleaseCondition {
    pub expiration_timestamp: u64,
    pub recipient_approval: bool,
    pub oracle_confirmation: bool,
    pub conditions: Vec<Condition>,
    pub operator: ConditionOperator,
    pub min_approvals: u32,
    pub current_approvals: u32,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum DisputeReason {
    AmountMismatch,
    NonDelivery,
    Fraud,
    Other,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum DisputeStatus {
    Open,
    InReview,
    Resolved,
    Cancelled,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum ResolutionOutcome {
    FavorSender,
    FavorRecipient,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct Dispute {
    pub disputer: Address,
    pub reason: DisputeReason,
    pub evidence_hash: BytesN<32>, // Store hash of evidence (e.g., IPFS CID hash)
    pub status: DisputeStatus,
    pub arbitrators: Vec<Address>,
    pub votes_sender: u32,
    pub votes_recipient: u32,
    pub voter_list: Vec<Address>,
    pub created_at: u64,
    pub resolved_at: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct Escrow {
    pub sender: Address,
    pub recipient: Address,
    pub amount: i128,
    pub deposited_amount: i128,
    pub released_amount: i128,
    pub refunded_amount: i128,
    pub asset: Asset,
    pub assets: Vec<Asset>,
    pub amounts: Map<Asset, i128>,
    pub deposited_amounts: Map<Asset, i128>,
    pub released_amounts: Map<Asset, i128>,
    pub refunded_amounts: Map<Asset, i128>,
    pub release_conditions: ReleaseCondition,
    pub status: EscrowStatus,
    pub created_at: u64,
    pub last_deposit_at: u64,
    pub release_timestamp: u64,
    pub refund_timestamp: u64,
    pub escrow_id: u64,
    pub memo: String,
    pub allow_partial_release: bool,
    pub multi_party_enabled: bool,
    pub kyc_compliant: bool,
    pub compliant: bool,
    pub milestones: Vec<Milestone>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct MultiPartyConfig {
    pub required_approvals: u32,
    pub approval_timeout: u64,
    pub whitelisted_approvers: Vec<Address>,
    pub approvals: Map<Address, bool>,
    pub finalized: bool,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum EventType {
    Created,
    Deposit,
    Approved,
    Released,
    PartialRelease,
    Refunded,
    PartialRefund,
}

#[derive(Clone, Debug)]
#[contracttype]
pub struct NotificationPayload {
    pub escrow_id: u64,
    pub event_type: EventType,
    pub amount: i128,
    pub timestamp: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct NotificationConfig {
    pub webhook_urls: Vec<String>,
    pub max_retries: u32,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct NotificationDelivery {
    pub escrow_id: u64,
    pub event_type: EventType,
    pub webhook_url: String,
    pub delivered_at: u64,
    pub attempts: u32,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct RecurringConfig {
    pub recipient: Address,
    pub asset: Asset,
    pub amount: i128,
    pub interval: u64,
    pub count: u32,
    pub auto_release: bool,
    pub expiration_window: u64,
    pub memo: String,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct RecurringEscrow {
    pub recurring_id: u64,
    pub sender: Address,
    pub config: RecurringConfig,
    pub next_run_at: u64,
    pub processed_count: u32,
    pub cancelled: bool,
    pub created_at: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct RecurringPayment {
    pub escrow_id: u64,
    pub processed_at: u64,
    pub sequence: u32,
}

const MAX_HOOKS: u32 = 10;
const DEFAULT_MAX_RETRIES: u32 = 2;

#[derive(Clone)]
#[contracttype]
pub enum DataKey {
    EscrowCounter,
    Escrow(u64),
    Admin,
    SupportedAssets,
    PlatformFeePercentage,
    ReentrancyGuard,
    ProcessingFeePercentage,
    FeeStructure,
    FeeWallet,
    ForexFeePercentage,
    ComplianceFlatFee,
    NetworkFlatFee,
    MinFee,
    MaxFee,
    EscrowApprovals(u64),
    KycEnabled,
    KycConfig,
    Dispute(u64),
    EscrowDelegation(u64, Address),
    DelegationHistory(u64),
    AnalyticsTotalVolume,
    AnalyticsEscrowCount(u32),
    AnalyticsAverageAmount,
    AnalyticsSuccessRate,
    AnalyticsLastUpdate,
    InsuranceConfig,
    EscrowInsurance(u64),
    ComplianceRules,
    EscrowComplianceOverride(u64),
    UserJurisdiction(Address),
    EscrowCancellationConfig(u64),
Recurring(u64),
    RecurringHistory(u64),
    NotificationHooks(u64),
    NotificationHistory(u64),
    RecurringCounter,
    Recurring(u64),
    RecurringHistory(u64),
}

#[contract]
pub struct PaymentEscrowContract;

#[contractimpl]
impl PaymentEscrowContract {
    pub fn init_escrow(env: Env, admin: Address) {
        if env.storage().instance().has(&DataKey::Admin) {
            panic!("Already initialized");
        }

        env.storage().instance().set(&DataKey::Admin, &admin);
        env.storage().instance().set(&DataKey::EscrowCounter, &0u64);
        env.storage()
            .instance()
            .set(&DataKey::RecurringCounter, &0u64);
        env.storage()
            .instance()
            .set(&DataKey::SupportedAssets, &Vec::<Asset>::new(&env));
        env.storage()
            .instance()
            .set(&DataKey::PlatformFeePercentage, &0i128);
        env.storage()
            .instance()
            .set(&DataKey::ProcessingFeePercentage, &0i128);
        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &false);
        env.storage().instance().set(&DataKey::KycEnabled, &false);

        upgradeable::init_version(&env);
    }

    pub fn add_supported_asset(env: Env, admin: Address, asset: Asset) {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            panic!("Unauthorized");
        }

        let mut assets: Vec<Asset> = env
            .storage()
            .instance()
            .get(&DataKey::SupportedAssets)
            .unwrap();
        assets.push_back(asset);
        env.storage()
            .instance()
            .set(&DataKey::SupportedAssets, &assets);
    }

    pub fn set_platform_fee(env: Env, admin: Address, fee_percentage: i128) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        if fee_percentage < 0 || fee_percentage > 10000 {
            return Err(Error::InvalidFeePercentage);
        }

        env.storage()
            .instance()
            .set(&DataKey::PlatformFeePercentage, &fee_percentage);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("fee_set"),
            0,
            &admin,
            fee_percentage,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("fee_set")),
        );

        Ok(())
    }

    pub fn get_platform_fee(env: Env) -> i128 {
        env.storage()
            .instance()
            .get(&DataKey::PlatformFeePercentage)
            .unwrap_or(0i128)
    }

    pub fn set_processing_fee(env: Env, admin: Address, fee_percentage: i128) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        if fee_percentage < 0 || fee_percentage > 10000 {
            return Err(Error::InvalidFeePercentage);
        }

        env.storage()
            .instance()
            .set(&DataKey::ProcessingFeePercentage, &fee_percentage);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("proc_fee"),
            0,
            &admin,
            fee_percentage,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("proc_fee")),
        );

        Ok(())
    }

    pub fn get_processing_fee(env: Env) -> i128 {
        env.storage()
            .instance()
            .get(&DataKey::ProcessingFeePercentage)
            .unwrap_or(0i128)
    }

    pub fn set_fee_wallet(env: Env, admin: Address, fee_wallet: Address) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        env.storage()
            .instance()
            .set(&DataKey::FeeWallet, &fee_wallet);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("fee_wal"),
            0,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("fee_wal")),
        );

        Ok(())
    }

    pub fn get_fee_wallet(env: Env) -> Option<Address> {
        env.storage().instance().get(&DataKey::FeeWallet)
    }

    pub fn set_forex_fee(env: Env, admin: Address, fee_percentage: i128) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        if fee_percentage < 0 || fee_percentage > 10000 {
            return Err(Error::InvalidAmount);
        }

        env.storage()
            .instance()
            .set(&DataKey::ForexFeePercentage, &fee_percentage);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("forex_f"),
            0,
            &admin,
            fee_percentage,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("forex_f")),
        );

        Ok(())
    }

    pub fn set_compliance_fee(env: Env, admin: Address, flat_fee: i128) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        if flat_fee < 0 {
            return Err(Error::InvalidAmount);
        }

        env.storage()
            .instance()
            .set(&DataKey::ComplianceFlatFee, &flat_fee);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("comp_fee"),
            0,
            &admin,
            flat_fee,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("comp_fee")),
        );

        Ok(())
    }

    pub fn set_fee_limits(
        env: Env,
        admin: Address,
        min_fee: i128,
        max_fee: i128,
    ) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        if min_fee < 0 || max_fee < min_fee {
            return Err(Error::InvalidAmount);
        }

        env.storage().instance().set(&DataKey::MinFee, &min_fee);
        env.storage().instance().set(&DataKey::MaxFee, &max_fee);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("fee_lim"),
            0,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("fee_lim")),
        );

        Ok(())
    }

    fn enforce_rate_limit(
        env: &Env,
        caller: &Address,
        function_type: FunctionType,
    ) -> Result<(), Error> {
        let admin_opt: Option<Address> = env.storage().instance().get(&DataKey::Admin);
        let admin = match admin_opt {
            Some(a) => a,
            None => return Ok(()),
        };
        let allowed = rate_limit::check_rate_limit(env, caller, function_type, &admin);
        if allowed {
            Ok(())
        } else {
            Err(Error::RateLimitExceeded)
        }
    }

    fn notify_external(env: &Env, payload: NotificationPayload) {
        let status = match payload.event_type {
            EventType::Created => symbol_short!("created"),
            EventType::Deposit => symbol_short!("deposit"),
            EventType::Approved => symbol_short!("approved"),
            EventType::Released => symbol_short!("released"),
            EventType::PartialRelease => symbol_short!("part_rel"),
            EventType::Refunded => symbol_short!("refunded"),
            EventType::PartialRefund => symbol_short!("part_ref"),
        };
        let actor = env.current_contract_address();
        events::emit(
            env,
            symbol_short!("escrow"),
            symbol_short!("notify"),
            payload.escrow_id,
            &actor,
            payload.amount,
            status.clone(),
            EventData::AdminAction(symbol_short!("notify")),
        );

        let hooks: Vec<NotificationConfig> = env
            .storage()
            .instance()
            .get(&DataKey::NotificationHooks(payload.escrow_id))
            .unwrap_or(Vec::new(env));
        let mut history: Vec<NotificationDelivery> = env
            .storage()
            .instance()
            .get(&DataKey::NotificationHistory(payload.escrow_id))
            .unwrap_or(Vec::new(env));
        for config in hooks.iter() {
            let retries = if config.max_retries == 0 {
                DEFAULT_MAX_RETRIES
            } else {
                config.max_retries
            };
            for url in config.webhook_urls.iter() {
                history.push_back(NotificationDelivery {
                    escrow_id: payload.escrow_id,
                    event_type: payload.event_type,
                    webhook_url: url,
                    delivered_at: payload.timestamp,
                    attempts: retries,
                });
                events::emit(
                    env,
                    symbol_short!("escrow"),
                    symbol_short!("hook"),
                    payload.escrow_id,
                    &actor,
                    payload.amount,
                    status.clone(),
                    EventData::AdminAction(symbol_short!("hook")),
                );
            }
        }
        env.storage()
            .instance()
            .set(&DataKey::NotificationHistory(payload.escrow_id), &history);
    }

    fn is_supported_asset(env: &Env, asset: &Asset) -> bool {
        let assets: Vec<Asset> = env
            .storage()
            .instance()
            .get(&DataKey::SupportedAssets)
            .unwrap_or(Vec::new(env));
        for supported_asset in assets.iter() {
            if supported_asset.code == asset.code && supported_asset.issuer == asset.issuer {
                return true;
            }
        }
        false
    }

    fn validate_notification_config(config: &NotificationConfig) -> Result<(), Error> {
        if config.webhook_urls.len() == 0 || config.webhook_urls.len() > MAX_HOOKS {
            return Err(Error::InvalidAsset);
        }

        for url in config.webhook_urls.iter() {
            if url.len() < 8 || url.len() > 256 {
                return Err(Error::InvalidAsset);
            }
        }

        Ok(())
    }

    fn empty_asset_amount_map(env: &Env) -> Map<Asset, i128> {
        Map::new(env)
    }

    fn calculate_fees(env: &Env, amount: i128) -> Result<FeeBreakdown, Error> {
        let platform_percentage = env
            .storage()
            .instance()
            .get(&DataKey::PlatformFeePercentage)
            .unwrap_or(0i128);
        let forex_percentage = env
            .storage()
            .instance()
            .get(&DataKey::ForexFeePercentage)
            .unwrap_or(0i128);
        let compliance_flat = env
            .storage()
            .instance()
            .get(&DataKey::ComplianceFlatFee)
            .unwrap_or(0i128);
        let network_flat = env
            .storage()
            .instance()
            .get(&DataKey::NetworkFlatFee)
            .unwrap_or(0i128);

        let platform_fee = amount
            .checked_mul(platform_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let forex_fee = amount
            .checked_mul(forex_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let mut total_fee = platform_fee
            .checked_add(forex_fee)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_add(compliance_flat)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_add(network_flat)
            .ok_or(Error::ArithmeticOverflow)?;

        let min_fee = env
            .storage()
            .instance()
            .get(&DataKey::MinFee)
            .unwrap_or(0i128);
        let max_fee = env
            .storage()
            .instance()
            .get(&DataKey::MaxFee)
            .unwrap_or(i128::MAX);

        if total_fee < min_fee {
            total_fee = min_fee;
        }
        if total_fee > max_fee {
            total_fee = max_fee;
        }

        if total_fee > amount {
            return Err(Error::FeeExceedsAmount);
        }

        Ok(FeeBreakdown {
            platform_fee,
            forex_fee,
            compliance_fee: compliance_flat,
            network_fee: network_flat,
            total_fee,
        })
    }

    pub fn get_fee_breakdown(env: Env, amount: i128) -> Result<FeeBreakdown, Error> {
        Self::calculate_fees(&env, amount)
    }

    pub fn configure_kyc(
        env: Env,
        admin: Address,
        oracle_address: Address,
        use_oracle: bool,
        proof_validity_period: u64,
    ) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        let config = KycConfig {
            admin: admin.clone(),
            oracle_address,
            use_oracle,
            proof_validity_period,
            last_check_ledger: 0,
        };

        env.storage().instance().set(&DataKey::KycConfig, &config);
        env.storage().instance().set(&DataKey::KycEnabled, &true);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("kyc_cfg"),
            0,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("kyc_cfg")),
        );

        Ok(())
    }

    pub fn register_compliance_rule(env: Env, admin: Address, rule: ComplianceRule) -> Result<(), Error> {
        admin.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        let mut rules: Vec<ComplianceRule> = env
            .storage()
            .instance()
            .get(&DataKey::ComplianceRules)
            .unwrap_or_else(|| Vec::new(&env));
        rules.push_back(rule);
        env.storage().instance().set(&DataKey::ComplianceRules, &rules);
        Ok(())
    }

    pub fn set_user_jurisdiction(env: Env, admin: Address, user: Address, country: i128) -> Result<(), Error> {
        admin.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        env.storage().instance().set(&DataKey::UserJurisdiction(user), &country);
        Ok(())
    }

    pub fn check_compliance(env: Env, escrow: Escrow) -> bool {
        let override_exists: bool = env.storage().instance().get(&DataKey::EscrowComplianceOverride(escrow.escrow_id)).unwrap_or(false);
        if override_exists {
            return true;
        }

        let rules_opt: Option<Vec<ComplianceRule>> = env.storage().instance().get(&DataKey::ComplianceRules);
        let rules = match rules_opt {
            Some(r) => r,
            None => return true,
        };

        for rule in rules.iter() {
            let rule_passed = match rule.rule_type {
                ComplianceRuleType::AmountThreshold => {
                    escrow.amount < rule.threshold
                }
                ComplianceRuleType::Jurisdiction => {
                    let sender_country: i128 = env.storage().instance().get(&DataKey::UserJurisdiction(escrow.sender.clone())).unwrap_or(0);
                    let recipient_country: i128 = env.storage().instance().get(&DataKey::UserJurisdiction(escrow.recipient.clone())).unwrap_or(0);
                    sender_country != rule.threshold && recipient_country != rule.threshold
                }
                ComplianceRuleType::RegulatoryRequirement => {
                    if escrow.amount >= rule.threshold {
                        escrow.kyc_compliant
                    } else {
                        true
                    }
                }
            };
            if !rule_passed {
                return false;
            }
        }
        true
    }

    pub fn admin_override_compliance(env: Env, admin: Address, escrow_id: u64) -> Result<(), Error> {
        admin.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;
        escrow.compliant = true;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);
        env.storage().instance().set(&DataKey::EscrowComplianceOverride(escrow_id), &true);
        Ok(())
    }

    pub fn add_to_whitelist(
        env: Env,
        admin: Address,
        account: Address,
        expiry: u64,
    ) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        let record = KycRecord {
            account: account.clone(),
            status: KycStatus::Verified,
            verified_at: env.ledger().timestamp(),
            issuer: admin.clone(),
            expiry,
        };

        env.storage()
            .persistent()
            .set(&KycDataKey::Whitelist(account.clone()), &record);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("kyc_add"),
            0,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AddressAction(symbol_short!("kyc_add"), account.clone()),
        );

        Ok(())
    }

    pub fn remove_from_whitelist(env: Env, admin: Address, account: Address) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        let record = KycRecord {
            account: account.clone(),
            status: KycStatus::Rejected,
            verified_at: env.ledger().timestamp(),
            issuer: admin.clone(),
            expiry: 0,
        };

        env.storage()
            .persistent()
            .set(&KycDataKey::Whitelist(account.clone()), &record);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("kyc_rem"),
            0,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AddressAction(symbol_short!("kyc_rem"), account.clone()),
        );

        Ok(())
    }

    pub fn add_trusted_issuer(env: Env, admin: Address, issuer: Address) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        env.storage()
            .persistent()
            .set(&KycDataKey::TrustedIssuer(issuer.clone()), &true);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("kyc_iss"),
            0,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AddressAction(symbol_short!("kyc_iss"), issuer),
        );

        Ok(())
    }

    pub fn get_kyc_status(env: Env, account: Address) -> KycStatus {
        let key = KycDataKey::Whitelist(account);
        let record: Option<KycRecord> = env.storage().persistent().get(&key);
        match record {
            Some(r) => r.status,
            None => KycStatus::Unknown,
        }
    }

    pub fn admin_override_kyc(env: Env, admin: Address, escrow_id: u64) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        escrow.kyc_compliant = true;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("kyc_ovr"),
            escrow_id,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("kyc_ovr")),
        );

        Ok(())
    }

    pub fn verify_kyc_proof(
        env: Env,
        account: Address,
        proof_signature: BytesN<64>,
        trusted_issuer: Address,
    ) -> Result<bool, Error> {
        let kyc_enabled: bool = env
            .storage()
            .instance()
            .get(&DataKey::KycEnabled)
            .unwrap_or(false);
        if !kyc_enabled {
            return Err(Error::KycNotConfigured);
        }

        let config: KycConfig = env
            .storage()
            .instance()
            .get(&DataKey::KycConfig)
            .ok_or(Error::KycNotConfigured)?;

        match kyc::verify_proof(
            &env,
            &account,
            &proof_signature,
            &trusted_issuer,
            config.proof_validity_period,
        ) {
            Ok(valid) => {
                if valid {
                    let record = KycRecord {
                        account: account.clone(),
                        status: KycStatus::Verified,
                        verified_at: env.ledger().timestamp(),
                        issuer: trusted_issuer,
                        expiry: if config.proof_validity_period > 0 {
                            env.ledger().timestamp() + config.proof_validity_period
                        } else {
                            0
                        },
                    };
                    env.storage()
                        .persistent()
                        .set(&KycDataKey::Whitelist(account.clone()), &record);

                    events::emit(
                        &env,
                        symbol_short!("escrow"),
                        symbol_short!("kyc_ok"),
                        0,
                        &account,
                        0,
                        symbol_short!("na"),
                        EventData::AddressAction(symbol_short!("kyc_ok"), account.clone()),
                    );
                }
                Ok(valid)
            }
            Err(_) => Err(Error::KycFailed),
        }
    }

    pub fn create_escrow(
        env: Env,
        sender: Address,
        recipient: Address,
        amount: i128,
        asset: Asset,
        expiration_timestamp: u64,
        memo: String,
    ) -> Result<u64, Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        sender.require_auth();
        Self::enforce_rate_limit(&env, &sender, FunctionType::Deposit)?;

        if amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        if sender == recipient {
            return Err(Error::SameSenderRecipient);
        }

        if !Self::is_supported_asset(&env, &asset) {
            return Err(Error::InvalidAsset);
        }

        let kyc_enabled: bool = env
            .storage()
            .instance()
            .get(&DataKey::KycEnabled)
            .unwrap_or(false);
        let mut kyc_compliant = false;

        if kyc_enabled {
            let config: KycConfig = env
                .storage()
                .instance()
                .get(&DataKey::KycConfig)
                .ok_or(Error::KycNotConfigured)?;

            let kyc_result = kyc::check_kyc(&env, &config, &sender, &recipient);

            match kyc_result {
                Ok(result) => {
                    if !result.sender_verified || !result.recipient_verified {
                        events::emit(
                            &env,
                            symbol_short!("escrow"),
                            symbol_short!("kyc_fail"),
                            0,
                            &sender,
                            0,
                            symbol_short!("na"),
                            EventData::PairAction(
                                symbol_short!("kyc_fail"),
                                sender.clone(),
                                recipient.clone(),
                            ),
                        );
                        return Err(Error::KycFailed);
                    }
                    kyc_compliant = true;

                    events::emit(
                        &env,
                        symbol_short!("escrow"),
                        symbol_short!("kyc_pass"),
                        0,
                        &sender,
                        0,
                        symbol_short!("na"),
                        EventData::PairAction(
                            symbol_short!("kyc_pass"),
                            sender.clone(),
                            recipient.clone(),
                        ),
                    );
                }
                Err(_) => {
                    return Err(Error::KycFailed);
                }
            }
        }

        let mut counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0u64);
        counter = counter.checked_add(1).ok_or(Error::CounterOverflow)?;

        let mut escrow_assets = Vec::new(&env);
        escrow_assets.push_back(asset.clone());
        let mut amounts = Self::empty_asset_amount_map(&env);
        amounts.set(asset.clone(), amount);
        let mut deposited_amounts = Self::empty_asset_amount_map(&env);
        deposited_amounts.set(asset.clone(), 0);
        let mut released_amounts = Self::empty_asset_amount_map(&env);
        released_amounts.set(asset.clone(), 0);
        let mut refunded_amounts = Self::empty_asset_amount_map(&env);
        refunded_amounts.set(asset.clone(), 0);

        let mut escrow = Escrow {
            sender: sender.clone(),
            recipient,
            amount,
            deposited_amount: 0,
            released_amount: 0,
            refunded_amount: 0,
            asset,
            assets: escrow_assets,
            amounts,
            deposited_amounts,
            released_amounts,
            refunded_amounts,
            release_conditions: ReleaseCondition {
                expiration_timestamp,
                recipient_approval: false,
                oracle_confirmation: false,
                conditions: Vec::new(&env),
                operator: ConditionOperator::And,
                min_approvals: 1,
                current_approvals: 0,
            },
            status: EscrowStatus::Pending,
            created_at: env.ledger().timestamp(),
            last_deposit_at: 0,
            release_timestamp: 0,
            refund_timestamp: 0,
            escrow_id: counter,
            memo,
            allow_partial_release: false,
            multi_party_enabled: false,
            kyc_compliant,
            compliant: true,
            milestones: Vec::new(&env),
        };

        // Auto-check compliance on creation
        let rules_opt: Option<Vec<ComplianceRule>> = env.storage().instance().get(&DataKey::ComplianceRules);
        if let Some(rules) = rules_opt {
            for rule in rules.iter() {
                let rule_passed = match rule.rule_type {
                    ComplianceRuleType::AmountThreshold => {
                        escrow.amount < rule.threshold
                    }
                    ComplianceRuleType::Jurisdiction => {
                        let sender_country: i128 = env.storage().instance().get(&DataKey::UserJurisdiction(escrow.sender.clone())).unwrap_or(0);
                        let recipient_country: i128 = env.storage().instance().get(&DataKey::UserJurisdiction(escrow.recipient.clone())).unwrap_or(0);
                        sender_country != rule.threshold && recipient_country != rule.threshold
                    }
                    ComplianceRuleType::RegulatoryRequirement => {
                        if escrow.amount >= rule.threshold {
                            escrow.kyc_compliant
                        } else {
                            true
                        }
                    }
                };

                if !rule_passed {
                    match rule.action {
                        ComplianceAction::Block => {
                            return Err(Error::NonCompliant);
                        }
                        ComplianceAction::Flag => {
                            escrow.compliant = false;
                        }
                    }
                }
            }
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(counter), &escrow);
        env.storage()
            .instance()
            .set(&DataKey::EscrowCounter, &counter);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("created"),
            counter,
            &escrow.sender,
            escrow.amount,
            symbol_short!("pending"),
            EventData::EscrowCreated(
                counter,
                escrow.sender.clone(),
                escrow.recipient.clone(),
                AssetRef {
                    code: escrow.asset.code.clone(),
                    issuer: escrow.asset.issuer.clone(),
                },
                escrow.amount,
            ),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id: counter,
                event_type: EventType::Created,
                amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        Ok(counter)
    }

    pub fn create_multi_asset_escrow(
        env: Env,
        sender: Address,
        recipient: Address,
        assets: Vec<Asset>,
        amounts: Map<Asset, i128>,
        expiration_timestamp: u64,
        memo: String,
    ) -> Result<u64, Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        sender.require_auth();
        Self::enforce_rate_limit(&env, &sender, FunctionType::Deposit)?;

        if sender == recipient {
            return Err(Error::SameSenderRecipient);
        }
        if assets.len() == 0 {
            return Err(Error::InvalidAsset);
        }

        let mut total_amount = 0i128;
        let mut deposited_amounts = Self::empty_asset_amount_map(&env);
        let mut released_amounts = Self::empty_asset_amount_map(&env);
        let mut refunded_amounts = Self::empty_asset_amount_map(&env);

        for asset in assets.iter() {
            if !Self::is_supported_asset(&env, &asset) {
                return Err(Error::InvalidAsset);
            }
            let amount = amounts.get(asset.clone()).ok_or(Error::InvalidAmount)?;
            if amount <= 0 {
                return Err(Error::InvalidAmount);
            }
            total_amount = total_amount
                .checked_add(amount)
                .ok_or(Error::ArithmeticOverflow)?;
            deposited_amounts.set(asset.clone(), 0);
            released_amounts.set(asset.clone(), 0);
            refunded_amounts.set(asset.clone(), 0);
        }

        let mut counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0u64);
        counter = counter.checked_add(1).ok_or(Error::CounterOverflow)?;

        let primary_asset = assets.get(0).unwrap();
        let primary_amount = amounts.get(primary_asset.clone()).unwrap();
        let escrow = Escrow {
            sender: sender.clone(),
            recipient,
            amount: total_amount,
            deposited_amount: 0,
            released_amount: 0,
            refunded_amount: 0,
            asset: primary_asset,
            assets,
            amounts,
            deposited_amounts,
            released_amounts,
            refunded_amounts,
            release_conditions: ReleaseCondition {
                expiration_timestamp,
                recipient_approval: false,
                oracle_confirmation: false,
                conditions: Vec::new(&env),
                operator: ConditionOperator::And,
                min_approvals: 1,
                current_approvals: 0,
            },
            status: EscrowStatus::Pending,
            created_at: env.ledger().timestamp(),
            last_deposit_at: 0,
            release_timestamp: 0,
            refund_timestamp: 0,
            escrow_id: counter,
            memo,
            allow_partial_release: false,
            multi_party_enabled: false,
            kyc_compliant: false,
            compliant: true,
            milestones: Vec::new(&env),
        };

        env.storage()
            .instance()
            .set(&DataKey::Escrow(counter), &escrow);
        env.storage()
            .instance()
            .set(&DataKey::EscrowCounter, &counter);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("created"),
            counter,
            &escrow.sender,
            primary_amount,
            symbol_short!("pending"),
            EventData::EscrowCreated(
                counter,
                escrow.sender.clone(),
                escrow.recipient.clone(),
                AssetRef {
                    code: escrow.asset.code.clone(),
                    issuer: escrow.asset.issuer.clone(),
                },
                primary_amount,
            ),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id: counter,
                event_type: EventType::Created,
                amount: total_amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        Ok(counter)
    }

    pub fn deposit(
        env: Env,
        escrow_id: u64,
        caller: Address,
        amount: i128,
        token_address: Address,
    ) -> Result<(), Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Deposit)?;

        if amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if !escrow.compliant {
            return Err(Error::NonCompliant);
        }

        if caller != escrow.sender {
            return Err(Error::WrongSender);
        }

        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded {
            return Err(Error::EscrowNotPending);
        }

        let new_deposited = escrow
            .deposited_amount
            .checked_add(amount)
            .ok_or(Error::DepositOverflow)?;

        if new_deposited > escrow.amount {
            return Err(Error::InsufficientAmount);
        }

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();

        token_client.transfer(&caller, &contract_address, &amount);

        escrow.deposited_amount = new_deposited;
        escrow
            .deposited_amounts
            .set(escrow.asset.clone(), new_deposited);
        escrow.last_deposit_at = env.ledger().timestamp();

        if escrow.deposited_amount == escrow.amount {
            escrow.status = EscrowStatus::Funded;
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        let deposit_status = if escrow.deposited_amount == escrow.amount {
            symbol_short!("funded")
        } else {
            symbol_short!("pending")
        };
        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("deposit"),
            escrow_id,
            &caller,
            amount,
            deposit_status,
            EventData::EscrowDeposited(escrow_id, amount, escrow.deposited_amount),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id,
                event_type: EventType::Deposit,
                amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        Ok(())
    }

    pub fn deposit_asset(
        env: Env,
        escrow_id: u64,
        caller: Address,
        asset: Asset,
        amount: i128,
        token_address: Address,
    ) -> Result<(), Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Deposit)?;

        if amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if !escrow.compliant {
            return Err(Error::NonCompliant);
        }

        if caller != escrow.sender {
            return Err(Error::WrongSender);
        }
        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded {
            return Err(Error::EscrowNotPending);
        }

        let required_amount = escrow
            .amounts
            .get(asset.clone())
            .ok_or(Error::InvalidAsset)?;
        let current_deposited = escrow.deposited_amounts.get(asset.clone()).unwrap_or(0i128);
        let new_asset_deposit = current_deposited
            .checked_add(amount)
            .ok_or(Error::DepositOverflow)?;
        if new_asset_deposit > required_amount {
            return Err(Error::InsufficientAmount);
        }

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();
        token_client.transfer(&caller, &contract_address, &amount);

        escrow
            .deposited_amounts
            .set(asset.clone(), new_asset_deposit);
        escrow.deposited_amount = escrow
            .deposited_amount
            .checked_add(amount)
            .ok_or(Error::DepositOverflow)?;
        escrow.last_deposit_at = env.ledger().timestamp();

        let mut fully_funded = true;
        for required_asset in escrow.assets.iter() {
            let required_asset: Asset = required_asset;
            let required: i128 = escrow.amounts.get(required_asset.clone()).unwrap_or(0i128);
            let deposited: i128 = escrow
                .deposited_amounts
                .get(required_asset.clone())
                .unwrap_or(0i128);
            if deposited < required {
                fully_funded = false;
                break;
            }
        }
        if fully_funded {
            escrow.status = EscrowStatus::Funded;
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("deposit"),
            escrow_id,
            &caller,
            amount,
            if fully_funded {
                symbol_short!("funded")
            } else {
                symbol_short!("pending")
            },
            EventData::EscrowDeposited(escrow_id, amount, escrow.deposited_amount),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id,
                event_type: EventType::Deposit,
                amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        Ok(())
    }

    pub fn get_escrow(env: Env, escrow_id: u64) -> Option<Escrow> {
        env.storage().instance().get(&DataKey::Escrow(escrow_id))
    }

    pub fn query_escrows_by_sender(
        env: Env,
        sender: Address,
        limit: u32,
        offset: u32,
    ) -> Vec<Escrow> {
        let mut results = Vec::new(&env);
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0u64);
        let mut skipped = 0u32;
        for id in 1..=counter {
            if let Some(escrow) = env
                .storage()
                .instance()
                .get::<_, Escrow>(&DataKey::Escrow(id))
            {
                if escrow.sender == sender {
                    if skipped < offset {
                        skipped += 1;
                    } else if results.len() < limit {
                        results.push_back(escrow);
                    }
                }
            }
        }
        results
    }

    pub fn query_escrows_by_recipient(
        env: Env,
        recipient: Address,
        limit: u32,
        offset: u32,
    ) -> Vec<Escrow> {
        let mut results = Vec::new(&env);
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0u64);
        let mut skipped = 0u32;
        for id in 1..=counter {
            if let Some(escrow) = env
                .storage()
                .instance()
                .get::<_, Escrow>(&DataKey::Escrow(id))
            {
                if escrow.recipient == recipient {
                    if skipped < offset {
                        skipped += 1;
                    } else if results.len() < limit {
                        results.push_back(escrow);
                    }
                }
            }
        }
        results
    }

    pub fn query_escrows_by_status(
        env: Env,
        status: EscrowStatus,
        limit: u32,
        offset: u32,
    ) -> Vec<Escrow> {
        let mut results = Vec::new(&env);
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0u64);
        let mut skipped = 0u32;
        for id in 1..=counter {
            if let Some(escrow) = env
                .storage()
                .instance()
                .get::<_, Escrow>(&DataKey::Escrow(id))
            {
                if escrow.status == status {
                    if skipped < offset {
                        skipped += 1;
                    } else if results.len() < limit {
                        results.push_back(escrow);
                    }
                }
            }
        }
        results
    }

    pub fn query_escrows_by_date_range(
        env: Env,
        start: u64,
        end: u64,
        limit: u32,
        offset: u32,
    ) -> Vec<Escrow> {
        let mut results = Vec::new(&env);
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0u64);
        let mut skipped = 0u32;
        for id in 1..=counter {
            if let Some(escrow) = env
                .storage()
                .instance()
                .get::<_, Escrow>(&DataKey::Escrow(id))
            {
                if escrow.created_at >= start && escrow.created_at <= end {
                    if skipped < offset {
                        skipped += 1;
                    } else if results.len() < limit {
                        results.push_back(escrow);
                    }
                }
            }
        }
        results
    }

    pub fn query_escrows_by_amount_range(
        env: Env,
        min_amount: i128,
        max_amount: i128,
        limit: u32,
        offset: u32,
    ) -> Vec<Escrow> {
        let mut results = Vec::new(&env);
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0u64);
        let mut skipped = 0u32;
        for id in 1..=counter {
            if let Some(escrow) = env
                .storage()
                .instance()
                .get::<_, Escrow>(&DataKey::Escrow(id))
            {
                if escrow.amount >= min_amount && escrow.amount <= max_amount {
                    if skipped < offset {
                        skipped += 1;
                    } else if results.len() < limit {
                        results.push_back(escrow);
                    }
                }
            }
        }
        results
    }

    pub fn register_notification_hook(
        env: Env,
        escrow_id: u64,
        caller: Address,
        config: NotificationConfig,
    ) -> Result<(), Error> {
        caller.require_auth();
        Self::validate_notification_config(&config)?;

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;
        let admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != escrow.recipient && caller != admin {
            return Err(Error::UnauthorizedCaller);
        }

        let mut hooks: Vec<NotificationConfig> = env
            .storage()
            .instance()
            .get(&DataKey::NotificationHooks(escrow_id))
            .unwrap_or(Vec::new(&env));
        if hooks.len() >= MAX_HOOKS {
            return Err(Error::InvalidStatus);
        }
        hooks.push_back(config);
        env.storage()
            .instance()
            .set(&DataKey::NotificationHooks(escrow_id), &hooks);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("hook_reg"),
            escrow_id,
            &caller,
            hooks.len() as i128,
            symbol_short!("active"),
            EventData::AdminAction(symbol_short!("hook_reg")),
        );

        Ok(())
    }

    pub fn get_notification_hooks(env: Env, escrow_id: u64) -> Vec<NotificationConfig> {
        env.storage()
            .instance()
            .get(&DataKey::NotificationHooks(escrow_id))
            .unwrap_or(Vec::new(&env))
    }

    pub fn get_notification_history(env: Env, escrow_id: u64) -> Vec<NotificationDelivery> {
        env.storage()
            .instance()
            .get(&DataKey::NotificationHistory(escrow_id))
            .unwrap_or(Vec::new(&env))
    }

    pub fn create_recurring_escrow(
        env: Env,
        sender: Address,
        config: RecurringConfig,
    ) -> Result<u64, Error> {
        sender.require_auth();
        if config.interval == 0 || config.count == 0 || config.amount <= 0 {
            return Err(Error::InvalidAmount);
        }
        if sender == config.recipient {
            return Err(Error::SameSenderRecipient);
        }
        if !Self::is_supported_asset(&env, &config.asset) {
            return Err(Error::InvalidAsset);
        }

        let mut counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::RecurringCounter)
            .unwrap_or(0u64);
        counter = counter.checked_add(1).ok_or(Error::CounterOverflow)?;

        let recurring = RecurringEscrow {
            recurring_id: counter,
            sender: sender.clone(),
            config,
            next_run_at: env.ledger().timestamp(),
            processed_count: 0,
            cancelled: false,
            created_at: env.ledger().timestamp(),
        };

        env.storage()
            .instance()
            .set(&DataKey::Recurring(counter), &recurring);
        env.storage()
            .instance()
            .set(&DataKey::RecurringCounter, &counter);
        env.storage().instance().set(
            &DataKey::RecurringHistory(counter),
            &Vec::<RecurringPayment>::new(&env),
        );

        Ok(counter)
    }

    pub fn process_recurring_payment(env: Env, recurring_id: u64) -> Result<u64, Error> {
        let mut recurring: RecurringEscrow = env
            .storage()
            .instance()
            .get(&DataKey::Recurring(recurring_id))
            .ok_or(Error::EscrowNotFound)?;
        if recurring.cancelled {
            return Err(Error::InvalidStatus);
        }
        if recurring.processed_count >= recurring.config.count {
            return Err(Error::InvalidStatus);
        }
        let now = env.ledger().timestamp();
        if now < recurring.next_run_at {
            return Err(Error::NotExpired);
        }

        let escrow_id = Self::create_escrow(
            env.clone(),
            recurring.sender.clone(),
            recurring.config.recipient.clone(),
            recurring.config.amount,
            recurring.config.asset.clone(),
            now.checked_add(recurring.config.expiration_window)
                .ok_or(Error::ArithmeticOverflow)?,
            recurring.config.memo.clone(),
        )?;

        if recurring.config.auto_release {
            let mut escrow: Escrow = env
                .storage()
                .instance()
                .get(&DataKey::Escrow(escrow_id))
                .ok_or(Error::EscrowNotFound)?;
            escrow.allow_partial_release = false;
            env.storage()
                .instance()
                .set(&DataKey::Escrow(escrow_id), &escrow);
        }

        recurring.processed_count = recurring
            .processed_count
            .checked_add(1)
            .ok_or(Error::ArithmeticOverflow)?;
        recurring.next_run_at = now
            .checked_add(recurring.config.interval)
            .ok_or(Error::ArithmeticOverflow)?;
        if recurring.processed_count >= recurring.config.count {
            recurring.cancelled = true;
        }
        env.storage()
            .instance()
            .set(&DataKey::Recurring(recurring_id), &recurring);

        let mut history: Vec<RecurringPayment> = env
            .storage()
            .instance()
            .get(&DataKey::RecurringHistory(recurring_id))
            .unwrap_or(Vec::new(&env));
        history.push_back(RecurringPayment {
            escrow_id,
            processed_at: now,
            sequence: recurring.processed_count,
        });
        env.storage()
            .instance()
            .set(&DataKey::RecurringHistory(recurring_id), &history);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("rec_pay"),
            escrow_id,
            &recurring.sender,
            recurring.config.amount,
            symbol_short!("created"),
            EventData::AdminAction(symbol_short!("rec_pay")),
        );

        Ok(escrow_id)
    }

    pub fn cancel_recurring_escrow(
        env: Env,
        recurring_id: u64,
        caller: Address,
    ) -> Result<(), Error> {
        caller.require_auth();
        let mut recurring: RecurringEscrow = env
            .storage()
            .instance()
            .get(&DataKey::Recurring(recurring_id))
            .ok_or(Error::EscrowNotFound)?;
        let admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != recurring.sender && caller != admin {
            return Err(Error::UnauthorizedCaller);
        }
        recurring.cancelled = true;
        env.storage()
            .instance()
            .set(&DataKey::Recurring(recurring_id), &recurring);
        Ok(())
    }

    pub fn get_recurring_escrow(env: Env, recurring_id: u64) -> Option<RecurringEscrow> {
        env.storage()
            .instance()
            .get(&DataKey::Recurring(recurring_id))
    }

    pub fn get_recurring_history(env: Env, recurring_id: u64) -> Vec<RecurringPayment> {
        env.storage()
            .instance()
            .get(&DataKey::RecurringHistory(recurring_id))
            .unwrap_or(Vec::new(&env))
    }

    pub fn approve_escrow(env: Env, escrow_id: u64, approver: Address) -> Result<(), Error> {
        approver.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if escrow.status != EscrowStatus::Funded {
            return Err(Error::InvalidStatus);
        }

        escrow.status = EscrowStatus::Approved;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("approved"),
            escrow_id,
            &approver,
            escrow.amount,
            symbol_short!("approved"),
            EventData::EscrowApproved(escrow_id),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id,
                event_type: EventType::Approved,
                amount: escrow.amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        Ok(())
    }
    pub fn extend_escrow(
        env: Env,
        escrow_id: u64,
        caller: Address,
        new_expiration: u64,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if caller != escrow.sender && caller != escrow.recipient {
            return Err(Error::Unauthorized);
        }

        let current_time = env.ledger().timestamp();
        if current_time > escrow.release_conditions.expiration_timestamp {
            return Err(Error::Expired);
        }

        if new_expiration <= current_time {
            return Err(Error::InvalidAmount);
        }

        escrow.release_conditions.expiration_timestamp = new_expiration;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("extended"),
            escrow_id,
            &caller,
            new_expiration as i128,
            symbol_short!("active"),
            EventData::EscrowExtended(escrow_id, new_expiration),
        );

        Ok(())
    }

    pub fn release_escrow(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
    ) -> Result<(), Error> {
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Release)?;

        let guard: bool = env
            .storage()
            .instance()
            .get(&DataKey::ReentrancyGuard)
            .unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if !escrow.compliant {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::NonCompliant);
        }

        if escrow.status != EscrowStatus::Approved && escrow.status != EscrowStatus::Funded {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::NotApproved);
        }

        if escrow.status == EscrowStatus::Released && !escrow.allow_partial_release {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyReleased);
        }

        if escrow.multi_party_enabled {
            let config_opt: Option<MultiPartyConfig> = env
                .storage()
                .instance()
                .get(&DataKey::EscrowApprovals(escrow_id));
            match config_opt {
                Some(config) => {
                    let current_time = env.ledger().timestamp();
                    if config.approval_timeout > 0 && current_time > config.approval_timeout {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::ApprovalExpired);
                    }
                    if config.approvals.len() < config.required_approvals {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::QuorumNotMet);
                    }
                }
                None => {
                    env.storage()
                        .instance()
                        .set(&DataKey::ReentrancyGuard, &false);
                    return Err(Error::QuorumNotMet);
                }
            }
        }

        let current_time = env.ledger().timestamp();
        if current_time > escrow.release_conditions.expiration_timestamp {
            escrow.status = EscrowStatus::Expired;
            env.storage()
                .instance()
                .set(&DataKey::Escrow(escrow_id), &escrow);
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::Expired);
        }

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.recipient && caller != stored_admin {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::Unauthorized);
        }

        if escrow.deposited_amount == 0 {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let available_amount = escrow
            .deposited_amount
            .checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if available_amount <= 0 {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let fee_percentage = Self::get_platform_fee(env.clone());
        let fee_amount = available_amount
            .checked_mul(fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let recipient_amount = available_amount
            .checked_sub(fee_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if recipient_amount <= 0 {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientAmount);
        }

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();

        token_client.transfer(&contract_address, &escrow.recipient, &recipient_amount);

        if fee_amount > 0 {
            token_client.transfer(&contract_address, &stored_admin, &fee_amount);
        }

        escrow.released_amount = escrow
            .released_amount
            .checked_add(available_amount)
            .ok_or(Error::ArithmeticOverflow)?;
        let current_asset_released = escrow
            .released_amounts
            .get(escrow.asset.clone())
            .unwrap_or(0i128);
        escrow.released_amounts.set(
            escrow.asset.clone(),
            current_asset_released
                .checked_add(available_amount)
                .ok_or(Error::ArithmeticOverflow)?,
        );
        escrow.status = EscrowStatus::Released;
        escrow.release_timestamp = current_time;

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        if escrow.multi_party_enabled {
            if let Some(mut config) = env
                .storage()
                .instance()
                .get::<_, MultiPartyConfig>(&DataKey::EscrowApprovals(escrow_id))
            {
                config.finalized = true;
                env.storage()
                    .instance()
                    .set(&DataKey::EscrowApprovals(escrow_id), &config);
            }
        }

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("released"),
            escrow_id,
            &caller,
            recipient_amount,
            symbol_short!("released"),
            EventData::EscrowReleased(escrow_id, recipient_amount),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id,
                event_type: EventType::Released,
                amount: recipient_amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn release_asset(
        env: Env,
        escrow_id: u64,
        caller: Address,
        asset: Asset,
        token_address: Address,
    ) -> Result<(), Error> {
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Release)?;

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if escrow.status != EscrowStatus::Approved && escrow.status != EscrowStatus::Funded {
            return Err(Error::NotApproved);
        }

        let current_time = env.ledger().timestamp();
        if current_time > escrow.release_conditions.expiration_timestamp {
            escrow.status = EscrowStatus::Expired;
            env.storage()
                .instance()
                .set(&DataKey::Escrow(escrow_id), &escrow);
            return Err(Error::Expired);
        }

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.recipient && caller != stored_admin && caller != escrow.sender {
            return Err(Error::UnauthorizedCaller);
        }

        let deposited = escrow.deposited_amounts.get(asset.clone()).unwrap_or(0i128);
        let released = escrow.released_amounts.get(asset.clone()).unwrap_or(0i128);
        let available_amount = deposited
            .checked_sub(released)
            .ok_or(Error::ArithmeticOverflow)?;
        if available_amount <= 0 {
            return Err(Error::InsufficientFunds);
        }

        let fee_breakdown = Self::calculate_fees(&env, available_amount)?;
        let recipient_amount = available_amount
            .checked_sub(fee_breakdown.total_fee)
            .ok_or(Error::ArithmeticOverflow)?;

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();
        token_client.transfer(&contract_address, &escrow.recipient, &recipient_amount);
        if fee_breakdown.total_fee > 0 {
            token_client.transfer(&contract_address, &stored_admin, &fee_breakdown.total_fee);
        }

        escrow.released_amounts.set(asset.clone(), deposited);
        escrow.released_amount = escrow
            .released_amount
            .checked_add(available_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        let mut all_released = true;
        for escrow_asset in escrow.assets.iter() {
            let escrow_asset: Asset = escrow_asset;
            let asset_deposited: i128 = escrow
                .deposited_amounts
                .get(escrow_asset.clone())
                .unwrap_or(0i128);
            let asset_released: i128 = escrow
                .released_amounts
                .get(escrow_asset.clone())
                .unwrap_or(0i128);
            if asset_deposited > asset_released {
                all_released = false;
                break;
            }
        }
        if all_released {
            escrow.status = EscrowStatus::Released;
            escrow.release_timestamp = current_time;
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("rel_ast"),
            escrow_id,
            &caller,
            recipient_amount,
            if all_released {
                symbol_short!("released")
            } else {
                symbol_short!("funded")
            },
            EventData::EscrowReleased(escrow_id, recipient_amount),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id,
                event_type: EventType::Released,
                amount: recipient_amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        Ok(())
    }

    pub fn release_partial(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
        release_amount: i128,
    ) -> Result<(), Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Release)?;

        if release_amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        let guard: bool = env
            .storage()
            .instance()
            .get(&DataKey::ReentrancyGuard)
            .unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if !escrow.allow_partial_release {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::PartialReleaseNotAllowed);
        }

        if escrow.status != EscrowStatus::Approved
            && escrow.status != EscrowStatus::Funded
            && escrow.status != EscrowStatus::Released
        {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InvalidStatus);
        }

        if escrow.multi_party_enabled {
            let config_opt: Option<MultiPartyConfig> = env
                .storage()
                .instance()
                .get(&DataKey::EscrowApprovals(escrow_id));
            match config_opt {
                Some(config) => {
                    let current_time = env.ledger().timestamp();
                    if config.approval_timeout > 0 && current_time > config.approval_timeout {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::ApprovalExpired);
                    }
                    if config.approvals.len() < config.required_approvals {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::QuorumNotMet);
                    }
                }
                None => {
                    env.storage()
                        .instance()
                        .set(&DataKey::ReentrancyGuard, &false);
                    return Err(Error::QuorumNotMet);
                }
            }
        }

        let current_time = env.ledger().timestamp();
        if current_time > escrow.release_conditions.expiration_timestamp {
            escrow.status = EscrowStatus::Expired;
            env.storage()
                .instance()
                .set(&DataKey::Escrow(escrow_id), &escrow);
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::Expired);
        }

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.recipient && caller != stored_admin {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::UnauthorizedCaller);
        }

        let available_amount = escrow
            .deposited_amount
            .checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if release_amount > available_amount {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let fee_percentage = Self::get_platform_fee(env.clone());
        let fee_amount = release_amount
            .checked_mul(fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let recipient_amount = release_amount
            .checked_sub(fee_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();

        token_client.transfer(&contract_address, &escrow.recipient, &recipient_amount);

        if fee_amount > 0 {
            token_client.transfer(&contract_address, &stored_admin, &fee_amount);
        }

        escrow.released_amount = escrow
            .released_amount
            .checked_add(release_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if escrow.released_amount >= escrow.deposited_amount {
            escrow.status = EscrowStatus::Released;
        }

        escrow.release_timestamp = current_time;

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        if escrow.multi_party_enabled {
            if let Some(mut config) = env
                .storage()
                .instance()
                .get::<_, MultiPartyConfig>(&DataKey::EscrowApprovals(escrow_id))
            {
                if escrow.released_amount >= escrow.deposited_amount {
                    config.finalized = true;
                    env.storage()
                        .instance()
                        .set(&DataKey::EscrowApprovals(escrow_id), &config);
                }
            }
        }

        let partial_status = if escrow.released_amount >= escrow.deposited_amount {
            symbol_short!("released")
        } else {
            symbol_short!("funded")
        };
        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("partial"),
            escrow_id,
            &caller,
            recipient_amount,
            partial_status,
            EventData::EscrowReleased(escrow_id, recipient_amount),
        );

        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn enable_partial_release(env: Env, escrow_id: u64, caller: Address) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if caller != escrow.sender {
            return Err(Error::Unauthorized);
        }

        escrow.allow_partial_release = true;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("part_enab"),
            escrow_id,
            &caller,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn add_condition(
        env: Env,
        escrow_id: u64,
        caller: Address,
        condition_type: ConditionType,
        required: bool,
        threshold_value: i128,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded {
            return Err(Error::InvalidStatus);
        }

        let condition = Condition {
            condition_type,
            required,
            verified: false,
            threshold_value,
        };

        escrow.release_conditions.conditions.push_back(condition);
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("cond_add"),
            escrow_id,
            &caller,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn set_condition_operator(
        env: Env,
        escrow_id: u64,
        caller: Address,
        operator: ConditionOperator,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        escrow.release_conditions.operator = operator;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("cond_op"),
            escrow_id,
            &caller,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn verify_conditions(
        env: Env,
        escrow_id: u64,
        proof_data: i128,
    ) -> Result<VerificationResult, Error> {
        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let current_time = env.ledger().timestamp();
        let mut failed_conditions = Vec::new(&env);
        let mut passed_count = 0;
        let mut required_count = 0;

        for i in 0..escrow.release_conditions.conditions.len() {
            let mut condition = escrow.release_conditions.conditions.get(i).unwrap();
            let condition_type_copy = condition.condition_type;
            let is_required = condition.required;

            if is_required {
                required_count += 1;
            }

            let verified = match condition.condition_type {
                ConditionType::Timestamp => {
                    current_time >= escrow.release_conditions.expiration_timestamp
                }
                ConditionType::Approval => {
                    escrow.release_conditions.current_approvals
                        >= escrow.release_conditions.min_approvals
                }
                ConditionType::OraclePrice => {
                    if proof_data > 0 {
                        proof_data >= condition.threshold_value
                    } else {
                        false
                    }
                }
                ConditionType::MultiSignature => {
                    escrow.release_conditions.current_approvals
                        >= escrow.release_conditions.min_approvals
                }
                ConditionType::KYCVerified => escrow.kyc_compliant,
            };

            condition.verified = verified;
            escrow.release_conditions.conditions.set(i, condition);

            if verified {
                passed_count += 1;
            } else if is_required {
                failed_conditions.push_back(condition_type_copy);
            }
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        let all_passed = match escrow.release_conditions.operator {
            ConditionOperator::And => {
                failed_conditions.is_empty()
                    && (required_count == 0 || passed_count >= required_count)
            }
            ConditionOperator::Or => passed_count > 0,
        };

        let result = VerificationResult {
            all_passed,
            failed_conditions,
        };

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("verified"),
            escrow_id,
            &env.current_contract_address(),
            0,
            if all_passed {
                symbol_short!("pass")
            } else {
                symbol_short!("fail")
            },
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(result)
    }

    pub fn add_approval(env: Env, escrow_id: u64, approver: Address) -> Result<(), Error> {
        approver.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if approver != stored_admin && approver != escrow.recipient && approver != escrow.sender {
            return Err(Error::Unauthorized);
        }

        escrow.release_conditions.current_approvals = escrow
            .release_conditions
            .current_approvals
            .checked_add(1)
            .unwrap_or(escrow.release_conditions.current_approvals);

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("approval"),
            escrow_id,
            &approver,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn set_min_approvals(
        env: Env,
        escrow_id: u64,
        caller: Address,
        min_approvals: u32,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        escrow.release_conditions.min_approvals = min_approvals;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("min_appr"),
            escrow_id,
            &caller,
            min_approvals as i128,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn refund_escrow(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
        reason: RefundReason,
    ) -> Result<(), Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Refund)?;

        let guard: bool = env
            .storage()
            .instance()
            .get(&DataKey::ReentrancyGuard)
            .unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::UnauthorizedRefund);
        }

        if escrow.status == EscrowStatus::Released {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyReleased);
        }

        if escrow.status == EscrowStatus::Refunded {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyRefunded);
        }

        if escrow.status != EscrowStatus::Pending
            && escrow.status != EscrowStatus::Funded
            && escrow.status != EscrowStatus::Approved
        {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InvalidStatus);
        }

        if escrow.multi_party_enabled {
            let config_opt: Option<MultiPartyConfig> = env
                .storage()
                .instance()
                .get(&DataKey::EscrowApprovals(escrow_id));
            match config_opt {
                Some(config) => {
                    let now = env.ledger().timestamp();
                    if config.approval_timeout > 0 && now > config.approval_timeout {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::ApprovalExpired);
                    }
                    if config.approvals.len() < config.required_approvals {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::QuorumNotMet);
                    }
                }
                None => {
                    env.storage()
                        .instance()
                        .set(&DataKey::ReentrancyGuard, &false);
                    return Err(Error::QuorumNotMet);
                }
            }
        }

        let current_time = env.ledger().timestamp();

        if reason == RefundReason::Expiration {
            if current_time <= escrow.release_conditions.expiration_timestamp {
                env.storage()
                    .instance()
                    .set(&DataKey::ReentrancyGuard, &false);
                return Err(Error::NotExpired);
            }
        }

        let available_for_refund = escrow
            .deposited_amount
            .checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_sub(escrow.refunded_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if available_for_refund <= 0 {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::NoFundsAvailable);
        }

        let processing_fee_percentage = Self::get_processing_fee(env.clone());
        let processing_fee = available_for_refund
            .checked_mul(processing_fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let refund_amount = available_for_refund
            .checked_sub(processing_fee)
            .ok_or(Error::ArithmeticOverflow)?;

        if refund_amount > 0 {
            let token_client = token::Client::new(&env, &token_address);
            let contract_address = env.current_contract_address();

            token_client.transfer(&contract_address, &escrow.sender, &refund_amount);

            if processing_fee > 0 {
                token_client.transfer(&contract_address, &stored_admin, &processing_fee);
            }
        }

        escrow.refunded_amount = escrow
            .refunded_amount
            .checked_add(available_for_refund)
            .ok_or(Error::ArithmeticOverflow)?;
        let current_asset_refunded = escrow
            .refunded_amounts
            .get(escrow.asset.clone())
            .unwrap_or(0i128);
        escrow.refunded_amounts.set(
            escrow.asset.clone(),
            current_asset_refunded
                .checked_add(available_for_refund)
                .ok_or(Error::ArithmeticOverflow)?,
        );
        escrow.status = EscrowStatus::Refunded;
        escrow.refund_timestamp = current_time;

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        if escrow.multi_party_enabled {
            if let Some(mut config) = env
                .storage()
                .instance()
                .get::<_, MultiPartyConfig>(&DataKey::EscrowApprovals(escrow_id))
            {
                config.finalized = true;
                env.storage()
                    .instance()
                    .set(&DataKey::EscrowApprovals(escrow_id), &config);
            }
        }

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("refunded"),
            escrow_id,
            &caller,
            refund_amount,
            symbol_short!("refunded"),
            EventData::EscrowRefunded(escrow_id, refund_amount),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id,
                event_type: EventType::Refunded,
                amount: refund_amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn refund_asset(
        env: Env,
        escrow_id: u64,
        caller: Address,
        asset: Asset,
        token_address: Address,
        reason: RefundReason,
    ) -> Result<(), Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Refund)?;

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::UnauthorizedRefund);
        }
        if escrow.status == EscrowStatus::Released {
            return Err(Error::AlreadyReleased);
        }
        if escrow.status != EscrowStatus::Pending
            && escrow.status != EscrowStatus::Funded
            && escrow.status != EscrowStatus::Approved
        {
            return Err(Error::InvalidStatus);
        }

        let current_time = env.ledger().timestamp();
        if reason == RefundReason::Expiration
            && current_time <= escrow.release_conditions.expiration_timestamp
        {
            return Err(Error::NotExpired);
        }

        let deposited = escrow.deposited_amounts.get(asset.clone()).unwrap_or(0i128);
        let released = escrow.released_amounts.get(asset.clone()).unwrap_or(0i128);
        let refunded = escrow.refunded_amounts.get(asset.clone()).unwrap_or(0i128);
        let available_for_refund = deposited
            .checked_sub(released)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_sub(refunded)
            .ok_or(Error::ArithmeticOverflow)?;
        if available_for_refund <= 0 {
            return Err(Error::NoFundsAvailable);
        }

        let processing_fee_percentage = Self::get_processing_fee(env.clone());
        let processing_fee = available_for_refund
            .checked_mul(processing_fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;
        let refund_amount = available_for_refund
            .checked_sub(processing_fee)
            .ok_or(Error::ArithmeticOverflow)?;

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();
        token_client.transfer(&contract_address, &escrow.sender, &refund_amount);
        if processing_fee > 0 {
            token_client.transfer(&contract_address, &stored_admin, &processing_fee);
        }

        escrow.refunded_amounts.set(
            asset.clone(),
            refunded
                .checked_add(available_for_refund)
                .ok_or(Error::ArithmeticOverflow)?,
        );
        escrow.refunded_amount = escrow
            .refunded_amount
            .checked_add(available_for_refund)
            .ok_or(Error::ArithmeticOverflow)?;

        let mut fully_processed = true;
        for escrow_asset in escrow.assets.iter() {
            let escrow_asset: Asset = escrow_asset;
            let asset_deposited: i128 = escrow
                .deposited_amounts
                .get(escrow_asset.clone())
                .unwrap_or(0i128);
            let asset_released: i128 = escrow
                .released_amounts
                .get(escrow_asset.clone())
                .unwrap_or(0i128);
            let asset_refunded: i128 = escrow
                .refunded_amounts
                .get(escrow_asset.clone())
                .unwrap_or(0i128);
            if asset_released + asset_refunded < asset_deposited {
                fully_processed = false;
                break;
            }
        }
        if fully_processed {
            escrow.status = EscrowStatus::Refunded;
            escrow.refund_timestamp = current_time;
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("ref_ast"),
            escrow_id,
            &caller,
            refund_amount,
            if fully_processed {
                symbol_short!("refunded")
            } else {
                symbol_short!("funded")
            },
            EventData::EscrowRefunded(escrow_id, refund_amount),
        );

        Self::notify_external(
            &env,
            NotificationPayload {
                escrow_id,
                event_type: EventType::Refunded,
                amount: refund_amount,
                timestamp: env.ledger().timestamp(),
            },
        );

        Ok(())
    }

    pub fn refund_partial(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
        refund_amount: i128,
        _reason: RefundReason,
    ) -> Result<(), Error> {
        if upgradeable::is_paused(&env) {
            return Err(Error::ContractPaused);
        }
        caller.require_auth();
        Self::enforce_rate_limit(&env, &caller, FunctionType::Refund)?;

        if refund_amount <= 0 {
            return Err(Error::InvalidRefundAmount);
        }

        let guard: bool = env
            .storage()
            .instance()
            .get(&DataKey::ReentrancyGuard)
            .unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::UnauthorizedRefund);
        }

        if escrow.status == EscrowStatus::Released {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyReleased);
        }

        if escrow.status != EscrowStatus::Pending
            && escrow.status != EscrowStatus::Funded
            && escrow.status != EscrowStatus::Approved
            && escrow.status != EscrowStatus::Refunded
        {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InvalidStatus);
        }

        if escrow.multi_party_enabled {
            let config_opt: Option<MultiPartyConfig> = env
                .storage()
                .instance()
                .get(&DataKey::EscrowApprovals(escrow_id));
            match config_opt {
                Some(config) => {
                    let now = env.ledger().timestamp();
                    if config.approval_timeout > 0 && now > config.approval_timeout {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::ApprovalExpired);
                    }
                    if config.approvals.len() < config.required_approvals {
                        env.storage()
                            .instance()
                            .set(&DataKey::ReentrancyGuard, &false);
                        return Err(Error::QuorumNotMet);
                    }
                }
                None => {
                    env.storage()
                        .instance()
                        .set(&DataKey::ReentrancyGuard, &false);
                    return Err(Error::QuorumNotMet);
                }
            }
        }

        let available_for_refund = escrow
            .deposited_amount
            .checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_sub(escrow.refunded_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if refund_amount > available_for_refund {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let processing_fee_percentage = Self::get_processing_fee(env.clone());
        let processing_fee = refund_amount
            .checked_mul(processing_fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let net_refund = refund_amount
            .checked_sub(processing_fee)
            .ok_or(Error::ArithmeticOverflow)?;

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();

        token_client.transfer(&contract_address, &escrow.sender, &net_refund);

        if processing_fee > 0 {
            token_client.transfer(&contract_address, &stored_admin, &processing_fee);
        }

        escrow.refunded_amount = escrow
            .refunded_amount
            .checked_add(refund_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        let current_time = env.ledger().timestamp();
        escrow.refund_timestamp = current_time;

        let total_processed = escrow
            .released_amount
            .checked_add(escrow.refunded_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if total_processed >= escrow.deposited_amount {
            escrow.status = EscrowStatus::Refunded;
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        if escrow.multi_party_enabled {
            if let Some(mut config) = env
                .storage()
                .instance()
                .get::<_, MultiPartyConfig>(&DataKey::EscrowApprovals(escrow_id))
            {
                if total_processed >= escrow.deposited_amount {
                    config.finalized = true;
                    env.storage()
                        .instance()
                        .set(&DataKey::EscrowApprovals(escrow_id), &config);
                }
            }
        }

        let refund_status = if escrow.refunded_amount >= escrow.deposited_amount {
            symbol_short!("refunded")
        } else {
            symbol_short!("funded")
        };
        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("ref_part"),
            escrow_id,
            &caller,
            net_refund,
            refund_status,
            EventData::EscrowRefunded(escrow_id, net_refund),
        );

        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn set_cancellation_config(
        env: Env,
        escrow_id: u64,
        caller: Address,
        config: CancellationConfig,
    ) -> Result<(), Error> {
        caller.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if caller != escrow.sender {
            return Err(Error::Unauthorized);
        }

        if config.penalty_percentage < 0 || config.penalty_percentage > 10000 {
            return Err(Error::InvalidFeePercentage);
        }

        if config.recipient_compensation < 0 || config.recipient_compensation > 10000 {
            return Err(Error::InvalidFeePercentage);
        }

        env.storage()
            .instance()
            .set(&DataKey::EscrowCancellationConfig(escrow_id), &config);

        Ok(())
    }

    pub fn cancel_escrow(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
        reason: String,
    ) -> Result<(), Error> {
        caller.require_auth();

        let guard: bool = env
            .storage()
            .instance()
            .get(&DataKey::ReentrancyGuard)
            .unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or({
                env.storage()
                    .instance()
                    .set(&DataKey::ReentrancyGuard, &false);
                Error::EscrowNotFound
            })?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            env.storage()
                .instance()
                .set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::Unauthorized);
        }

        match escrow.status {
            EscrowStatus::Released
            | EscrowStatus::Refunded
            | EscrowStatus::Expired
            | EscrowStatus::Cancelled => {
                env.storage()
                    .instance()
                    .set(&DataKey::ReentrancyGuard, &false);
                return Err(Error::InvalidStatus);
            }
            _ => {}
        }

        let deposited = escrow.deposited_amount;

        if deposited > 0 {
            let config_opt: Option<CancellationConfig> = env
                .storage()
                .instance()
                .get(&DataKey::EscrowCancellationConfig(escrow_id));

            let token_client = token::Client::new(&env, &token_address);
            let contract_address = env.current_contract_address();

            if let Some(config) = config_opt {
                let penalty = deposited
                    .checked_mul(config.penalty_percentage)
                    .ok_or(Error::ArithmeticOverflow)?
                    .checked_div(10000)
                    .ok_or(Error::ArithmeticOverflow)?;

                let recipient_comp = deposited
                    .checked_mul(config.recipient_compensation)
                    .ok_or(Error::ArithmeticOverflow)?
                    .checked_div(10000)
                    .ok_or(Error::ArithmeticOverflow)?;

                let sender_refund = deposited
                    .checked_sub(penalty)
                    .ok_or(Error::ArithmeticOverflow)?
                    .checked_sub(recipient_comp)
                    .ok_or(Error::ArithmeticOverflow)?;

                if recipient_comp > 0 {
                    token_client.transfer(&contract_address, &escrow.recipient, &recipient_comp);
                }

                if penalty > 0 {
                    let fee_wallet_opt: Option<Address> =
                        env.storage().instance().get(&DataKey::FeeWallet);
                    let penalty_dest = fee_wallet_opt.unwrap_or(escrow.sender.clone());
                    token_client.transfer(&contract_address, &penalty_dest, &penalty);
                }

                if sender_refund > 0 {
                    token_client.transfer(&contract_address, &escrow.sender, &sender_refund);
                }
            } else {
                token_client.transfer(&contract_address, &escrow.sender, &deposited);
            }
        }

        // Silence unused variable warning for reason
        let _ = reason;

        escrow.status = EscrowStatus::Cancelled;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("cancel"),
            escrow_id,
            &caller,
            deposited,
            symbol_short!("cancel"),
            EventData::AdminAction(symbol_short!("cancel")),
        );

        env.storage()
            .instance()
            .set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn get_cancellation_config(env: Env, escrow_id: u64) -> Option<CancellationConfig> {
        env.storage()
            .instance()
            .get(&DataKey::EscrowCancellationConfig(escrow_id))
    }

    pub fn setup_multi_party_approval(
        env: Env,
        escrow_id: u64,
        caller: Address,
        approvers: Vec<Address>,
        required_approvals: u32,
        approval_timeout: u64,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded {
            return Err(Error::InvalidStatus);
        }

        if escrow.multi_party_enabled {
            return Err(Error::InvalidStatus);
        }

        if required_approvals == 0 || required_approvals > approvers.len() {
            return Err(Error::InvalidStatus);
        }

        let config = MultiPartyConfig {
            required_approvals,
            approval_timeout,
            whitelisted_approvers: approvers,
            approvals: Map::new(&env),
            finalized: false,
        };

        escrow.multi_party_enabled = true;
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);
        env.storage()
            .instance()
            .set(&DataKey::EscrowApprovals(escrow_id), &config);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("mp_setup"),
            escrow_id,
            &caller,
            required_approvals as i128,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn add_approver(
        env: Env,
        escrow_id: u64,
        caller: Address,
        new_approver: Address,
    ) -> Result<(), Error> {
        caller.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        let mut config: MultiPartyConfig = env
            .storage()
            .instance()
            .get(&DataKey::EscrowApprovals(escrow_id))
            .ok_or(Error::ConditionsNotMet)?;

        if config.finalized {
            return Err(Error::EscrowFinalized);
        }

        for i in 0..config.whitelisted_approvers.len() {
            if config.whitelisted_approvers.get(i).unwrap() == new_approver {
                return Err(Error::AlreadyApproved);
            }
        }

        config.whitelisted_approvers.push_back(new_approver.clone());
        env.storage()
            .instance()
            .set(&DataKey::EscrowApprovals(escrow_id), &config);

        env.events()
            .publish((symbol_short!("appr_add"), escrow_id), new_approver);

        Ok(())
    }

    pub fn remove_approver(
        env: Env,
        escrow_id: u64,
        caller: Address,
        approver: Address,
    ) -> Result<(), Error> {
        caller.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        let mut config: MultiPartyConfig = env
            .storage()
            .instance()
            .get(&DataKey::EscrowApprovals(escrow_id))
            .ok_or(Error::ConditionsNotMet)?;

        if config.finalized {
            return Err(Error::EscrowFinalized);
        }

        let mut found = false;
        let mut new_approvers = Vec::new(&env);
        for i in 0..config.whitelisted_approvers.len() {
            let addr = config.whitelisted_approvers.get(i).unwrap();
            if addr == approver {
                found = true;
            } else {
                new_approvers.push_back(addr);
            }
        }

        if !found {
            return Err(Error::ApproverNotWhitelisted);
        }

        if new_approvers.len() < config.required_approvals {
            return Err(Error::InvalidStatus);
        }

        config.approvals.remove(approver.clone());
        config.whitelisted_approvers = new_approvers;
        env.storage()
            .instance()
            .set(&DataKey::EscrowApprovals(escrow_id), &config);

        env.events()
            .publish((symbol_short!("appr_rem"), escrow_id), approver);

        Ok(())
    }

    pub fn multi_party_approve(env: Env, escrow_id: u64, approver: Address) -> Result<bool, Error> {
        approver.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if !escrow.multi_party_enabled {
            return Err(Error::ConditionsNotMet);
        }

        let mut config: MultiPartyConfig = env
            .storage()
            .instance()
            .get(&DataKey::EscrowApprovals(escrow_id))
            .ok_or(Error::ConditionsNotMet)?;

        if config.finalized {
            return Err(Error::EscrowFinalized);
        }

        let current_time = env.ledger().timestamp();
        if config.approval_timeout > 0 && current_time > config.approval_timeout {
            return Err(Error::ApprovalExpired);
        }

        let mut is_whitelisted = false;
        for i in 0..config.whitelisted_approvers.len() {
            if config.whitelisted_approvers.get(i).unwrap() == approver {
                is_whitelisted = true;
                break;
            }
        }

        if !is_whitelisted {
            return Err(Error::ApproverNotWhitelisted);
        }

        if config.approvals.contains_key(approver.clone()) {
            return Err(Error::AlreadyApproved);
        }

        config.approvals.set(approver.clone(), true);
        let approval_count = config.approvals.len();
        let quorum_met = approval_count >= config.required_approvals;

        env.storage()
            .instance()
            .set(&DataKey::EscrowApprovals(escrow_id), &config);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("mp_appr"),
            escrow_id,
            &approver,
            approval_count as i128,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        if quorum_met {
            events::emit(
                &env,
                symbol_short!("escrow"),
                symbol_short!("quorum"),
                escrow_id,
                &env.current_contract_address(),
                approval_count as i128,
                symbol_short!("na"),
                EventData::AdminAction(symbol_short!("na")),
            );
        }

        Ok(quorum_met)
    }

    pub fn revoke_approval(env: Env, escrow_id: u64, approver: Address) -> Result<(), Error> {
        approver.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if !escrow.multi_party_enabled {
            return Err(Error::ConditionsNotMet);
        }

        let mut config: MultiPartyConfig = env
            .storage()
            .instance()
            .get(&DataKey::EscrowApprovals(escrow_id))
            .ok_or(Error::ConditionsNotMet)?;

        if config.finalized {
            return Err(Error::EscrowFinalized);
        }

        if !config.approvals.contains_key(approver.clone()) {
            return Err(Error::ApprovalNotFound);
        }

        config.approvals.remove(approver.clone());
        env.storage()
            .instance()
            .set(&DataKey::EscrowApprovals(escrow_id), &config);

        env.events()
            .publish((symbol_short!("mp_revok"), escrow_id), approver);

        Ok(())
    }

    pub fn get_multi_party_status(env: Env, escrow_id: u64) -> Option<MultiPartyConfig> {
        env.storage()
            .instance()
            .get(&DataKey::EscrowApprovals(escrow_id))
    }

    pub fn raise_dispute(
        env: Env,
        escrow_id: u64,
        disputer: Address,
        reason: DisputeReason,
        evidence_hash: BytesN<32>,
    ) -> Result<(), Error> {
        disputer.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if disputer != escrow.sender && disputer != escrow.recipient {
            return Err(Error::UnauthorizedCaller);
        }

        if env.storage().instance().has(&DataKey::Dispute(escrow_id)) {
            return Err(Error::AlreadyDisputed);
        }

        if escrow.status != EscrowStatus::Funded && escrow.status != EscrowStatus::Approved {
            return Err(Error::NotApproved);
        }

        let admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        let mut arbitrators = Vec::new(&env);
        arbitrators.push_back(admin);

        let dispute = Dispute {
            disputer: disputer.clone(),
            reason,
            evidence_hash,
            status: DisputeStatus::Open,
            arbitrators,
            votes_sender: 0,
            votes_recipient: 0,
            voter_list: Vec::new(&env),
            created_at: env.ledger().timestamp(),
            resolved_at: 0,
        };

        env.storage()
            .instance()
            .set(&DataKey::Dispute(escrow_id), &dispute);

        escrow.status = EscrowStatus::Disputed;

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        env.events()
            .publish((symbol_short!("disp_rais"), escrow_id), (disputer, reason));

        Ok(())
    }

    pub fn vote_on_dispute(
        env: Env,
        escrow_id: u64,
        voter: Address,
        outcome: ResolutionOutcome,
    ) -> Result<(), Error> {
        voter.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let mut dispute: Dispute = env
            .storage()
            .instance()
            .get(&DataKey::Dispute(escrow_id))
            .ok_or(Error::DisputeNotFound)?;

        if dispute.status != DisputeStatus::Open && dispute.status != DisputeStatus::InReview {
            return Err(Error::InvalidStatus);
        }

        let mut is_arbitrator = false;
        for i in 0..dispute.arbitrators.len() {
            if dispute.arbitrators.get(i).unwrap() == voter {
                is_arbitrator = true;
                break;
            }
        }

        if !is_arbitrator {
            return Err(Error::NotArbitrator);
        }

        let mut already_voted = false;
        for i in 0..dispute.voter_list.len() {
            if dispute.voter_list.get(i).unwrap() == voter {
                already_voted = true;
                break;
            }
        }
        if already_voted {
            return Err(Error::AlreadyVoted);
        }

        match outcome {
            ResolutionOutcome::FavorSender => dispute.votes_sender += 1,
            ResolutionOutcome::FavorRecipient => dispute.votes_recipient += 1,
        }

        dispute.voter_list.push_back(voter.clone());
        dispute.status = DisputeStatus::InReview;

        let total_arbitrators = dispute.arbitrators.len();
        let majority = (total_arbitrators / 2) + 1;

        if dispute.votes_sender >= majority as u32 {
            Self::finalize_dispute_internal(
                &env,
                &mut escrow,
                &mut dispute,
                ResolutionOutcome::FavorSender,
            )?;
        } else if dispute.votes_recipient >= majority as u32 {
            Self::finalize_dispute_internal(
                &env,
                &mut escrow,
                &mut dispute,
                ResolutionOutcome::FavorRecipient,
            )?;
        } else {
            env.storage()
                .instance()
                .set(&DataKey::Dispute(escrow_id), &dispute);
        }

        env.events()
            .publish((symbol_short!("disp_vote"), escrow_id), (voter, outcome));

        Ok(())
    }

    pub fn resolve_dispute(
        env: Env,
        escrow_id: u64,
        caller: Address,
        outcome: ResolutionOutcome,
    ) -> Result<(), Error> {
        caller.require_auth();

        let admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != admin {
            return Err(Error::Unauthorized);
        }

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let mut dispute: Dispute = env
            .storage()
            .instance()
            .get(&DataKey::Dispute(escrow_id))
            .ok_or(Error::DisputeNotFound)?;

        Self::finalize_dispute_internal(&env, &mut escrow, &mut dispute, outcome)
    }

    fn finalize_dispute_internal(
        env: &Env,
        escrow: &mut Escrow,
        dispute: &mut Dispute,
        outcome: ResolutionOutcome,
    ) -> Result<(), Error> {
        dispute.status = DisputeStatus::Resolved;
        dispute.resolved_at = env.ledger().timestamp();

        env.storage()
            .instance()
            .set(&DataKey::Dispute(escrow.escrow_id), &*dispute);

        match outcome {
            ResolutionOutcome::FavorSender => {
                escrow.status = EscrowStatus::Funded; // Can be refunded
            }
            ResolutionOutcome::FavorRecipient => {
                escrow.status = EscrowStatus::Approved; // Can be released
            }
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow.escrow_id), &*escrow);

        env.events()
            .publish((symbol_short!("disp_res"), escrow.escrow_id), outcome);

        Ok(())
    }

    pub fn get_dispute(env: Env, escrow_id: u64) -> Option<Dispute> {
        env.storage().instance().get(&DataKey::Dispute(escrow_id))
    }

    // ── Rate Limit Configuration ─────────────────────────────────────

    pub fn set_rate_limit_config(
        env: Env,
        caller: Address,
        function_type: rate_limit::FunctionType,
        config: rate_limit::RateLimitConfig,
    ) -> Result<(), Error> {
        caller.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != stored_admin {
            return Err(Error::UnauthorizedCaller);
        }
        rate_limit::set_config(&env, function_type, config);
        Ok(())
    }

    pub fn get_rate_limit_config(
        env: Env,
        function_type: rate_limit::FunctionType,
    ) -> Option<rate_limit::RateLimitConfig> {
        rate_limit::get_config(&env, function_type)
    }

    pub fn set_global_rate_limit_config(
        env: Env,
        caller: Address,
        function_type: rate_limit::FunctionType,
        config: rate_limit::RateLimitConfig,
    ) -> Result<(), Error> {
        caller.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != stored_admin {
            return Err(Error::UnauthorizedCaller);
        }
        rate_limit::set_global_config(&env, function_type, config);
        Ok(())
    }

    pub fn get_global_rate_limit_config(
        env: Env,
        function_type: rate_limit::FunctionType,
    ) -> Option<rate_limit::RateLimitConfig> {
        rate_limit::get_global_config(&env, function_type)
    }

    pub fn set_rate_limit_exemption(
        env: Env,
        caller: Address,
        address: Address,
        exempt: bool,
    ) -> Result<(), Error> {
        caller.require_auth();
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != stored_admin {
            return Err(Error::UnauthorizedCaller);
        }
        rate_limit::set_exemption(&env, &address, exempt);
        Ok(())
    }

    pub fn is_rate_limit_exempt(env: Env, address: Address) -> bool {
        rate_limit::is_exempt(&env, &address)
    }

    // ── Delegation Functions (#132) ────────────────────────────────────

    pub fn delegate_escrow(
        env: Env,
        escrow_id: u64,
        caller: Address,
        delegate: Address,
        permissions: DelegationPermissions,
    ) -> Result<(), Error> {
        caller.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        let delegation_key = DataKey::EscrowDelegation(escrow_id, delegate.clone());
        if env.storage().instance().has(&delegation_key) {
            return Err(Error::AlreadyApproved);
        }

        let entry = DelegationEntry {
            delegate: delegate.clone(),
            permissions,
            delegated_at: env.ledger().timestamp(),
        };

        env.storage()
            .instance()
            .set(&delegation_key, &entry);

        let mut history: Vec<DelegationEntry> = env
            .storage()
            .instance()
            .get(&DataKey::DelegationHistory(escrow_id))
            .unwrap_or(Vec::new(&env));
        history.push_back(entry);
        env.storage()
            .instance()
            .set(&DataKey::DelegationHistory(escrow_id), &history);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("delegate"),
            escrow_id,
            &caller,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn revoke_delegation(
        env: Env,
        escrow_id: u64,
        caller: Address,
        delegate: Address,
    ) -> Result<(), Error> {
        caller.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        let delegation_key = DataKey::EscrowDelegation(escrow_id, delegate.clone());
        if !env.storage().instance().has(&delegation_key) {
            return Err(Error::ApprovalNotFound);
        }

        env.storage().instance().remove(&delegation_key);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("revoke_d"),
            escrow_id,
            &caller,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn get_delegation(
        env: Env,
        escrow_id: u64,
        delegate: Address,
    ) -> Option<DelegationEntry> {
        env.storage()
            .instance()
            .get(&DataKey::EscrowDelegation(escrow_id, delegate))
    }

    pub fn get_delegation_history(env: Env, escrow_id: u64) -> Vec<DelegationEntry> {
        env.storage()
            .instance()
            .get(&DataKey::DelegationHistory(escrow_id))
            .unwrap_or(Vec::new(&env))
    }

    fn check_delegated_permission(
        env: &Env,
        escrow_id: u64,
        caller: &Address,
        permission_check: fn(&DelegationPermissions) -> bool,
    ) -> Result<bool, Error> {
        let delegation_key = DataKey::EscrowDelegation(escrow_id, caller.clone());
        if let Some(entry) = env.storage().instance().get::<_, DelegationEntry>(&delegation_key) {
            Ok(permission_check(&entry.permissions))
        } else {
            Ok(false)
        }
    }

    // ── Insurance Functions (#131) ─────────────────────────────────────

    pub fn set_insurance_config(
        env: Env,
        admin: Address,
        config: InsuranceConfig,
    ) -> Result<(), Error> {
        admin.require_auth();

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        env.storage()
            .instance()
            .set(&DataKey::InsuranceConfig, &config);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("ins_cfg"),
            0,
            &admin,
            0,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn get_insurance_config(env: Env) -> Option<InsuranceConfig> {
        env.storage().instance().get(&DataKey::InsuranceConfig)
    }

    pub fn insure_escrow(
        env: Env,
        escrow_id: u64,
        caller: Address,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if caller != escrow.sender {
            return Err(Error::Unauthorized);
        }

        if env.storage().instance().has(&DataKey::EscrowInsurance(escrow_id)) {
            return Err(Error::AlreadyApproved);
        }

        let config: InsuranceConfig = env
            .storage()
            .instance()
            .get(&DataKey::InsuranceConfig)
            .ok_or(Error::ConditionsNotMet)?;

        if !config.enabled {
            return Err(Error::ConditionsNotMet);
        }

        let premium = escrow
            .amount
            .checked_mul(config.premium_rate)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let coverage = if config.coverage_limit > 0 && config.coverage_limit < escrow.amount {
            config.coverage_limit
        } else {
            escrow.amount
        };

        let insurance = EscrowInsurance {
            insured: true,
            premium,
            coverage,
            claimed: false,
            claim_reason: None,
        };

        env.storage()
            .instance()
            .set(&DataKey::EscrowInsurance(escrow_id), &insurance);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("insured"),
            escrow_id,
            &caller,
            premium,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn claim_insurance(
        env: Env,
        escrow_id: u64,
        caller: Address,
        reason: String,
    ) -> Result<(), Error> {
        caller.require_auth();

        let escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let mut insurance: EscrowInsurance = env
            .storage()
            .instance()
            .get(&DataKey::EscrowInsurance(escrow_id))
            .ok_or(Error::ConditionsNotMet)?;

        if insurance.claimed {
            return Err(Error::AlreadyReleased);
        }

        if caller != escrow.recipient && caller != escrow.sender {
            return Err(Error::Unauthorized);
        }

        if reason.len() == 0 {
            return Err(Error::InvalidAmount);
        }

        insurance.claimed = true;
        insurance.claim_reason = Some(reason);

        env.storage()
            .instance()
            .set(&DataKey::EscrowInsurance(escrow_id), &insurance);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("ins_clm"),
            escrow_id,
            &caller,
            insurance.coverage,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn get_escrow_insurance(env: Env, escrow_id: u64) -> Option<EscrowInsurance> {
        env.storage().instance().get(&DataKey::EscrowInsurance(escrow_id))
    }

    // ── Milestone Functions (#129) ─────────────────────────────────────

    pub fn complete_milestone(
        env: Env,
        escrow_id: u64,
        milestone_index: u32,
        caller: Address,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        if milestone_index >= escrow.milestones.len() {
            return Err(Error::InvalidStatus);
        }

        let mut milestone = escrow.milestones.get(milestone_index).unwrap();
        if milestone.completed {
            return Err(Error::AlreadyApproved);
        }

        milestone.completed = true;
        milestone.completed_by = Some(caller.clone());
        escrow.milestones.set(milestone_index, milestone);

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("ms_comp"),
            escrow_id,
            &caller,
            milestone_index as i128,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn approve_milestone(
        env: Env,
        escrow_id: u64,
        milestone_index: u32,
        approver: Address,
    ) -> Result<(), Error> {
        approver.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        if approver != escrow.recipient {
            return Err(Error::Unauthorized);
        }

        if milestone_index >= escrow.milestones.len() {
            return Err(Error::InvalidStatus);
        }

        let milestone = escrow.milestones.get(milestone_index).unwrap();
        if !milestone.completed {
            return Err(Error::NotApproved);
        }
        if milestone.approved {
            return Err(Error::AlreadyApproved);
        }

        let milestone_amount = milestone.amount;
        let mut milestone_mut = milestone;
        milestone_mut.approved = true;
        milestone_mut.approved_by = Some(approver.clone());
        escrow.milestones.set(milestone_index, milestone_mut);

        escrow.released_amount = escrow
            .released_amount
            .checked_add(milestone_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        let total_released = escrow.released_amount;
        if total_released >= escrow.deposited_amount {
            escrow.status = EscrowStatus::Released;
        }

        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("ms_appr"),
            escrow_id,
            &approver,
            milestone_amount,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    pub fn get_milestones(env: Env, escrow_id: u64) -> Vec<Milestone> {
        let escrow_opt: Option<Escrow> = env.storage().instance().get(&DataKey::Escrow(escrow_id));
        match escrow_opt {
            Some(e) => e.milestones,
            None => Vec::new(&env),
        }
    }

    pub fn add_milestone(
        env: Env,
        escrow_id: u64,
        caller: Address,
        description: String,
        amount: i128,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env
            .storage()
            .instance()
            .get(&DataKey::Escrow(escrow_id))
            .ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded {
            return Err(Error::InvalidStatus);
        }

        if amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        let milestone = Milestone {
            description,
            amount,
            completed: false,
            approved: false,
            completed_by: None,
            approved_by: None,
        };

        escrow.milestones.push_back(milestone);
        env.storage()
            .instance()
            .set(&DataKey::Escrow(escrow_id), &escrow);

        events::emit(
            &env,
            symbol_short!("escrow"),
            symbol_short!("ms_add"),
            escrow_id,
            &caller,
            amount,
            symbol_short!("na"),
            EventData::AdminAction(symbol_short!("na")),
        );

        Ok(())
    }

    // ── Analytics Functions (#130) ─────────────────────────────────────

    pub fn get_total_escrow_volume(env: Env) -> i128 {
        let mut total: i128 = 0;
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0);
        for i in 1..=counter {
            if let Some(escrow) = env.storage().instance().get::<_, Escrow>(&DataKey::Escrow(i)) {
                total = total.checked_add(escrow.amount).unwrap_or(total);
            }
        }
        env.storage()
            .instance()
            .set(&DataKey::AnalyticsTotalVolume, &total);
        total
    }

    pub fn get_escrow_count_by_status(env: Env, status: EscrowStatus) -> u64 {
        let mut count: u64 = 0;
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0);
        for i in 1..=counter {
            if let Some(escrow) = env.storage().instance().get::<_, Escrow>(&DataKey::Escrow(i)) {
                if escrow.status == status {
                    count += 1;
                }
            }
        }
        count
    }

    pub fn get_average_escrow_amount(env: Env) -> i128 {
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0);
        if counter == 0 {
            return 0;
        }
        let mut total: i128 = 0;
        for i in 1..=counter {
            if let Some(escrow) = env.storage().instance().get::<_, Escrow>(&DataKey::Escrow(i)) {
                total = total.checked_add(escrow.amount).unwrap_or(total);
            }
        }
        total / (counter as i128)
    }

    pub fn get_success_rate(env: Env) -> i128 {
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0);
        if counter == 0 {
            return 0;
        }
        let mut completed: u64 = 0;
        for i in 1..=counter {
            if let Some(escrow) = env.storage().instance().get::<_, Escrow>(&DataKey::Escrow(i)) {
                if escrow.status == EscrowStatus::Released {
                    completed += 1;
                }
            }
        }
        (completed as i128) * 10000 / (counter as i128)
    }

    pub fn get_user_statistics(env: Env, user: Address) -> EscrowAnalytics {
        let counter: u64 = env
            .storage()
            .instance()
            .get(&DataKey::EscrowCounter)
            .unwrap_or(0);
        let mut total_volume: i128 = 0;
        let mut total_escrows: u64 = 0;
        let mut completed: u64 = 0;
        let mut refunded: u64 = 0;
        let mut disputed: u64 = 0;

        for i in 1..=counter {
            if let Some(escrow) = env.storage().instance().get::<_, Escrow>(&DataKey::Escrow(i)) {
                if escrow.sender == user || escrow.recipient == user {
                    total_escrows += 1;
                    total_volume = total_volume.checked_add(escrow.amount).unwrap_or(total_volume);
                    match escrow.status {
                        EscrowStatus::Released => completed += 1,
                        EscrowStatus::Refunded | EscrowStatus::Expired => refunded += 1,
                        EscrowStatus::Disputed => disputed += 1,
                        _ => {}
                    }
                }
            }
        }

        let average = if total_escrows > 0 {
            total_volume / (total_escrows as i128)
        } else {
            0
        };

        let success_rate = if total_escrows > 0 {
            (completed as i128) * 10000 / (total_escrows as i128)
        } else {
            0
        };

        EscrowAnalytics {
            total_volume,
            total_escrows,
            completed_escrows: completed,
            refunded_escrows: refunded,
            disputed_escrows: disputed,
            average_amount: average,
            success_rate,
            last_updated: env.ledger().timestamp(),
        }
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::{
        testutils::{Address as _, Ledger},
        token,
    };

    fn create_token_contract<'a>(
        env: &Env,
        admin: &Address,
    ) -> (token::Client<'a>, token::StellarAssetClient<'a>) {
        let contract_address = env.register_stellar_asset_contract_v2(admin.clone());
        (
            token::Client::new(env, &contract_address.address()),
            token::StellarAssetClient::new(env, &contract_address.address()),
        )
    }

    #[test]
    fn test_init_escrow() {
        let env = Env::default();
        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        env.mock_all_auths();

        client.init_escrow(&admin);
    }

    #[test]
    fn test_create_escrow() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: issuer.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment for services"),
        );
        assert_eq!(escrow_id, 1);

        let escrow = client.get_escrow(&escrow_id);
        assert!(escrow.is_some());

        let escrow_data = escrow.unwrap();
        assert_eq!(escrow_data.amount, 1000);
        assert_eq!(escrow_data.deposited_amount, 0);
        assert_eq!(escrow_data.released_amount, 0);
        assert_eq!(escrow_data.refunded_amount, 0);
        assert_eq!(escrow_data.sender, sender);
        assert_eq!(escrow_data.recipient, recipient);
        assert_eq!(escrow_data.status, EscrowStatus::Pending);
        assert_eq!(escrow_data.created_at, 1000);
        assert_eq!(escrow_data.allow_partial_release, false);
    }

    #[test]
    fn test_deposit_full_amount() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test payment"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.deposited_amount, 1000);
        assert_eq!(escrow.status, EscrowStatus::Funded);
        assert_eq!(escrow.last_deposit_at, 1000);
    }

    #[test]
    fn test_deposit_partial_amounts() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test payment"),
        );

        client.deposit(&escrow_id, &sender, &400, &token.address);
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.deposited_amount, 400);
        assert_eq!(escrow.status, EscrowStatus::Pending);

        client.deposit(&escrow_id, &sender, &600, &token.address);
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.deposited_amount, 1000);
        assert_eq!(escrow.status, EscrowStatus::Funded);
    }

    #[test]
    fn test_deposit_wrong_sender() {
        let env = Env::default();
        env.mock_all_auths();

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let wrong_sender = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&wrong_sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        let result = client.try_deposit(&escrow_id, &wrong_sender, &1000, &token.address);
        assert_eq!(result, Err(Ok(Error::WrongSender)));
    }

    #[test]
    fn test_deposit_exceeds_amount() {
        let env = Env::default();
        env.mock_all_auths();

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        let result = client.try_deposit(&escrow_id, &sender, &1500, &token.address);
        assert_eq!(result, Err(Ok(Error::InsufficientAmount)));
    }

    #[test]
    fn test_approve_and_release_with_deposit() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        client.approve_escrow(&escrow_id, &admin);
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Approved);

        let recipient_balance_before = token.balance(&recipient);
        client.release_escrow(&escrow_id, &recipient, &token.address);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Released);

        let recipient_balance_after = token.balance(&recipient);
        assert_eq!(recipient_balance_after - recipient_balance_before, 1000);
    }

    #[test]
    fn test_refund_after_expiration() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let sender_balance_before = token.balance(&sender);
        client.refund_escrow(
            &escrow_id,
            &sender,
            &token.address,
            &RefundReason::Expiration,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Refunded);
        assert_eq!(escrow.refunded_amount, 1000);

        let sender_balance_after = token.balance(&sender);
        assert_eq!(sender_balance_after - sender_balance_before, 1000);
    }

    #[test]
    fn test_set_platform_fee() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.init_escrow(&admin);

        client.set_platform_fee(&admin, &250);

        let fee = client.get_platform_fee();
        assert_eq!(fee, 250);
    }

    #[test]
    fn test_release_with_fee() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);
        client.set_platform_fee(&admin, &250);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);
        client.approve_escrow(&escrow_id, &admin);

        let recipient_balance_before = token.balance(&recipient);
        let admin_balance_before = token.balance(&admin);

        client.release_escrow(&escrow_id, &recipient, &token.address);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Released);
        assert_eq!(escrow.released_amount, 1000);

        let recipient_balance_after = token.balance(&recipient);
        let admin_balance_after = token.balance(&admin);

        let fee = 1000 * 250 / 10000;
        assert_eq!(
            recipient_balance_after - recipient_balance_before,
            1000 - fee
        );
        assert_eq!(admin_balance_after - admin_balance_before, fee);
    }

    #[test]
    fn test_partial_release() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);
        client.approve_escrow(&escrow_id, &admin);
        client.enable_partial_release(&escrow_id, &sender);

        let recipient_balance_before = token.balance(&recipient);

        client.release_partial(&escrow_id, &recipient, &token.address, &400);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.released_amount, 400);

        let recipient_balance_after = token.balance(&recipient);
        assert_eq!(recipient_balance_after - recipient_balance_before, 400);

        client.release_partial(&escrow_id, &recipient, &token.address, &600);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.released_amount, 1000);
        assert_eq!(escrow.status, EscrowStatus::Released);
    }

    #[test]
    fn test_release_unauthorized_caller() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let unauthorized = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);
        client.approve_escrow(&escrow_id, &admin);

        let result = client.try_release_escrow(&escrow_id, &unauthorized, &token.address);
        assert_eq!(result, Err(Ok(Error::Unauthorized)));
    }

    #[test]
    fn test_refund_with_processing_fee() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);
        client.set_processing_fee(&admin, &100);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let sender_balance_before = token.balance(&sender);
        let admin_balance_before = token.balance(&admin);

        client.refund_escrow(
            &escrow_id,
            &sender,
            &token.address,
            &RefundReason::Expiration,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Refunded);
        assert_eq!(escrow.refunded_amount, 1000);

        let sender_balance_after = token.balance(&sender);
        let admin_balance_after = token.balance(&admin);

        let fee = 1000 * 100 / 10000;
        assert_eq!(sender_balance_after - sender_balance_before, 1000 - fee);
        assert_eq!(admin_balance_after - admin_balance_before, fee);
    }

    #[test]
    fn test_refund_by_admin() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        let sender_balance_before = token.balance(&sender);
        client.refund_escrow(
            &escrow_id,
            &admin,
            &token.address,
            &RefundReason::AdminAction,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Refunded);

        let sender_balance_after = token.balance(&sender);
        assert_eq!(sender_balance_after - sender_balance_before, 1000);
    }

    #[test]
    fn test_partial_refund() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        let sender_balance_before = token.balance(&sender);

        client.refund_partial(
            &escrow_id,
            &sender,
            &token.address,
            &400,
            &RefundReason::Dispute,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.refunded_amount, 400);

        let sender_balance_after = token.balance(&sender);
        assert_eq!(sender_balance_after - sender_balance_before, 400);

        client.refund_partial(
            &escrow_id,
            &sender,
            &token.address,
            &600,
            &RefundReason::Dispute,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.refunded_amount, 1000);
        assert_eq!(escrow.status, EscrowStatus::Refunded);
    }

    #[test]
    fn test_refund_unauthorized() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let unauthorized = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let result = client.try_refund_escrow(
            &escrow_id,
            &unauthorized,
            &token.address,
            &RefundReason::Expiration,
        );
        assert_eq!(result, Err(Ok(Error::UnauthorizedRefund)));
    }

    #[test]
    fn test_refund_already_released() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let (token, token_admin) = create_token_contract(&env, &admin);
        token_admin.mint(&sender, &5000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);
        client.approve_escrow(&escrow_id, &admin);
        client.release_escrow(&escrow_id, &recipient, &token.address);

        let result = client.try_refund_escrow(
            &escrow_id,
            &sender,
            &token.address,
            &RefundReason::Expiration,
        );
        assert_eq!(result, Err(Ok(Error::AlreadyReleased)));
    }

    #[test]
    fn test_add_condition() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.add_condition(
            &escrow_id,
            &sender,
            &ConditionType::OraclePrice,
            &true,
            &100,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.release_conditions.conditions.len(), 1);
    }

    #[test]
    fn test_verify_conditions_timestamp() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::Timestamp, &true, &0);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let result = client.verify_conditions(&escrow_id, &0);
        assert_eq!(result.all_passed, true);
    }

    #[test]
    fn test_verify_conditions_approval() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::Approval, &true, &0);
        client.set_min_approvals(&escrow_id, &sender, &2);

        client.add_approval(&escrow_id, &admin);
        client.add_approval(&escrow_id, &recipient);

        let result = client.verify_conditions(&escrow_id, &0);
        assert_eq!(result.all_passed, true);
    }

    #[test]
    fn test_verify_conditions_oracle_price() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.add_condition(
            &escrow_id,
            &sender,
            &ConditionType::OraclePrice,
            &true,
            &100,
        );

        let result = client.verify_conditions(&escrow_id, &150);
        assert_eq!(result.all_passed, true);

        let result_fail = client.verify_conditions(&escrow_id, &50);
        assert_eq!(result_fail.all_passed, false);
    }

    #[test]
    fn test_verify_conditions_and_operator() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::Timestamp, &true, &0);
        client.add_condition(
            &escrow_id,
            &sender,
            &ConditionType::OraclePrice,
            &true,
            &100,
        );
        client.set_condition_operator(&escrow_id, &sender, &ConditionOperator::And);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let result = client.verify_conditions(&escrow_id, &150);
        assert_eq!(result.all_passed, true);
    }

    #[test]
    fn test_verify_conditions_or_operator() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::Timestamp, &true, &0);
        client.add_condition(
            &escrow_id,
            &sender,
            &ConditionType::OraclePrice,
            &true,
            &100,
        );
        client.set_condition_operator(&escrow_id, &sender, &ConditionOperator::Or);

        let result = client.verify_conditions(&escrow_id, &150);
        assert_eq!(result.all_passed, true);
    }

    #[test]
    fn test_multi_signature_approval() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Test"),
        );

        client.set_min_approvals(&escrow_id, &sender, &3);

        client.add_approval(&escrow_id, &admin);
        client.add_approval(&escrow_id, &sender);
        client.add_approval(&escrow_id, &recipient);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.release_conditions.current_approvals, 3);
    }

    #[test]
    fn test_calculate_fees() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.init_escrow(&admin);

        client.set_platform_fee(&admin, &250);
        client.set_forex_fee(&admin, &100);
        client.set_compliance_fee(&admin, &10);

        let breakdown = client.get_fee_breakdown(&1000);

        let expected_platform = 1000 * 250 / 10000;
        let expected_forex = 1000 * 100 / 10000;
        let expected_compliance = 10;
        let expected_total = expected_platform + expected_forex + expected_compliance;

        assert_eq!(breakdown.platform_fee, expected_platform);
        assert_eq!(breakdown.forex_fee, expected_forex);
        assert_eq!(breakdown.compliance_fee, expected_compliance);
        assert_eq!(breakdown.total_fee, expected_total);
    }

    #[test]
    fn test_fee_limits() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.init_escrow(&admin);

        client.set_platform_fee(&admin, &100);
        client.set_fee_limits(&admin, &50, &200);

        let breakdown_low = client.get_fee_breakdown(&100);
        assert_eq!(breakdown_low.total_fee, 50);

        let breakdown_high = client.get_fee_breakdown(&100000);
        assert_eq!(breakdown_high.total_fee, 200);
    }

    #[test]
    fn test_set_fee_wallet() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let fee_wallet = Address::generate(&env);

        client.init_escrow(&admin);
        client.set_fee_wallet(&admin, &fee_wallet);

        let stored_wallet = client.get_fee_wallet();
        assert!(stored_wallet.is_some());
        assert_eq!(stored_wallet.unwrap(), fee_wallet);
    }

    #[test]
    fn test_fee_exceeds_amount() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.init_escrow(&admin);

        client.set_platform_fee(&admin, &9000);
        client.set_forex_fee(&admin, &2000);

        let result = client.try_get_fee_breakdown(&1000);
        assert_eq!(result, Err(Ok(Error::FeeExceedsAmount)));
    }

    #[test]
    fn test_forex_fee_configuration() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.init_escrow(&admin);

        client.set_forex_fee(&admin, &150);

        let breakdown = client.get_fee_breakdown(&1000);
        let expected_forex = 1000 * 150 / 10000;
        assert_eq!(breakdown.forex_fee, expected_forex);
    }

    #[test]
    fn test_compliance_flat_fee() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.init_escrow(&admin);

        client.set_compliance_fee(&admin, &25);

        let breakdown = client.get_fee_breakdown(&1000);
        assert_eq!(breakdown.compliance_fee, 25);
    }

    // === Multi-Party Approval Tests ===

    fn setup_escrow_for_multi_party(
        env: &Env,
    ) -> (
        PaymentEscrowContractClient,
        Address,
        Address,
        Address,
        u64,
        token::Client,
        Address,
    ) {
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let admin = Address::generate(env);
        let sender = Address::generate(env);
        let recipient = Address::generate(env);

        let (token, token_admin) = create_token_contract(env, &admin);
        token_admin.mint(&sender, &10000);

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(env, &contract_id);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &5000,
            &asset,
            &10000,
            &String::from_str(env, "Multi-party test"),
        );

        client.deposit(&escrow_id, &sender, &5000, &token.address);

        let token_address = token.address.clone();
        (
            client,
            admin,
            sender,
            recipient,
            escrow_id,
            token,
            token_address,
        )
    }

    #[test]
    fn test_setup_multi_party_approval() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let approver1 = Address::generate(&env);
        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(approver1.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.multi_party_enabled, true);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.required_approvals, 2);
        assert_eq!(config.approval_timeout, 5000);
        assert_eq!(config.whitelisted_approvers.len(), 3);
        assert_eq!(config.approvals.len(), 0);
        assert_eq!(config.finalized, false);
    }

    #[test]
    fn test_setup_multi_party_invalid_quorum() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        // required_approvals > approvers count
        let result =
            client.try_setup_multi_party_approval(&escrow_id, &admin, &approvers, &5, &5000);
        assert_eq!(result, Err(Ok(Error::InvalidStatus)));

        // required_approvals == 0
        let result =
            client.try_setup_multi_party_approval(&escrow_id, &admin, &approvers, &0, &5000);
        assert_eq!(result, Err(Ok(Error::InvalidStatus)));
    }

    #[test]
    fn test_setup_multi_party_duplicate_rejected() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        // Cannot setup again
        let result =
            client.try_setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);
        assert_eq!(result, Err(Ok(Error::InvalidStatus)));
    }

    #[test]
    fn test_multi_party_approve_single() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(admin.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let quorum_met = client.multi_party_approve(&escrow_id, &sender);
        assert_eq!(quorum_met, false);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 1);
    }

    #[test]
    fn test_multi_party_quorum_met() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(admin.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let result1 = client.multi_party_approve(&escrow_id, &sender);
        assert_eq!(result1, false);

        let result2 = client.multi_party_approve(&escrow_id, &recipient);
        assert_eq!(result2, true);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 2);
    }

    #[test]
    fn test_multi_party_duplicate_approval_rejected() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);

        let result = client.try_multi_party_approve(&escrow_id, &sender);
        assert_eq!(result, Err(Ok(Error::AlreadyApproved)));
    }

    #[test]
    fn test_multi_party_non_whitelisted_rejected() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let outsider = Address::generate(&env);
        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let result = client.try_multi_party_approve(&escrow_id, &outsider);
        assert_eq!(result, Err(Ok(Error::ApproverNotWhitelisted)));
    }

    #[test]
    fn test_multi_party_approval_expired() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        // Advance time beyond approval timeout
        env.ledger().with_mut(|li| {
            li.timestamp = 6000;
        });

        let result = client.try_multi_party_approve(&escrow_id, &sender);
        assert_eq!(result, Err(Ok(Error::ApprovalExpired)));
    }

    #[test]
    fn test_multi_party_no_timeout() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        // timeout = 0 means no timeout
        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &0);

        env.ledger().with_mut(|li| {
            li.timestamp = 999999;
        });

        // Should still work with no timeout
        let result = client.multi_party_approve(&escrow_id, &sender);
        assert_eq!(result, false);
    }

    #[test]
    fn test_revoke_approval() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(admin.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 2);

        client.revoke_approval(&escrow_id, &sender);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 1);
    }

    #[test]
    fn test_revoke_approval_not_found() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let result = client.try_revoke_approval(&escrow_id, &sender);
        assert_eq!(result, Err(Ok(Error::ApprovalNotFound)));
    }

    #[test]
    fn test_revoke_after_finalized_rejected() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        client.approve_escrow(&escrow_id, &admin);
        client.release_escrow(&escrow_id, &recipient, &token_addr);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.finalized, true);

        let result = client.try_revoke_approval(&escrow_id, &sender);
        assert_eq!(result, Err(Ok(Error::EscrowFinalized)));
    }

    #[test]
    fn test_release_blocked_without_quorum() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(admin.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);
        client.approve_escrow(&escrow_id, &admin);

        // Only 1 approval, need 2
        client.multi_party_approve(&escrow_id, &sender);

        let result = client.try_release_escrow(&escrow_id, &recipient, &token_addr);
        assert_eq!(result, Err(Ok(Error::QuorumNotMet)));
    }

    #[test]
    fn test_release_succeeds_with_quorum() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(admin.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);
        client.approve_escrow(&escrow_id, &admin);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        let recipient_balance_before = token.balance(&recipient);
        client.release_escrow(&escrow_id, &recipient, &token_addr);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Released);

        let recipient_balance_after = token.balance(&recipient);
        assert_eq!(recipient_balance_after - recipient_balance_before, 5000);
    }

    #[test]
    fn test_refund_blocked_without_quorum() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);

        let result = client.try_refund_escrow(
            &escrow_id,
            &sender,
            &token_addr,
            &RefundReason::SenderRequest,
        );
        assert_eq!(result, Err(Ok(Error::QuorumNotMet)));
    }

    #[test]
    fn test_refund_succeeds_with_quorum() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        let sender_balance_before = token.balance(&sender);
        client.refund_escrow(
            &escrow_id,
            &sender,
            &token_addr,
            &RefundReason::SenderRequest,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Refunded);

        let sender_balance_after = token.balance(&sender);
        assert_eq!(sender_balance_after - sender_balance_before, 5000);
    }

    #[test]
    fn test_add_approver_dynamic() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let new_approver = Address::generate(&env);
        client.add_approver(&escrow_id, &admin, &new_approver);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.whitelisted_approvers.len(), 3);

        // New approver can now approve
        let result = client.multi_party_approve(&escrow_id, &new_approver);
        assert_eq!(result, false);
    }

    #[test]
    fn test_add_approver_duplicate_rejected() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let result = client.try_add_approver(&escrow_id, &admin, &sender);
        assert_eq!(result, Err(Ok(Error::AlreadyApproved)));
    }

    #[test]
    fn test_remove_approver() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let approver3 = Address::generate(&env);
        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(approver3.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.remove_approver(&escrow_id, &admin, &approver3);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.whitelisted_approvers.len(), 2);

        // Removed approver can no longer approve
        let result = client.try_multi_party_approve(&escrow_id, &approver3);
        assert_eq!(result, Err(Ok(Error::ApproverNotWhitelisted)));
    }

    #[test]
    fn test_remove_approver_clears_existing_approval() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let approver3 = Address::generate(&env);
        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(approver3.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &approver3);
        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 1);

        client.remove_approver(&escrow_id, &admin, &approver3);
        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 0);
    }

    #[test]
    fn test_remove_approver_violating_quorum_rejected() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        // 2 approvers, 2 required -> can't remove any
        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        let result = client.try_remove_approver(&escrow_id, &admin, &sender);
        assert_eq!(result, Err(Ok(Error::InvalidStatus)));
    }

    #[test]
    fn test_approve_on_non_multi_party_escrow_rejected() {
        let env = Env::default();
        let (client, _admin, sender, _recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        // No multi-party setup done
        let result = client.try_multi_party_approve(&escrow_id, &sender);
        assert_eq!(result, Err(Ok(Error::ConditionsNotMet)));
    }

    #[test]
    fn test_revoke_on_non_multi_party_escrow_rejected() {
        let env = Env::default();
        let (client, _admin, sender, _recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let result = client.try_revoke_approval(&escrow_id, &sender);
        assert_eq!(result, Err(Ok(Error::ConditionsNotMet)));
    }

    #[test]
    fn test_approve_after_finalized_rejected() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(admin.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        client.approve_escrow(&escrow_id, &admin);
        client.release_escrow(&escrow_id, &recipient, &token_addr);

        // After release, config is finalized
        let result = client.try_multi_party_approve(&escrow_id, &admin);
        assert_eq!(result, Err(Ok(Error::EscrowFinalized)));
    }

    #[test]
    fn test_multi_party_full_flow_2_of_3() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let compliance_officer = Address::generate(&env);
        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(compliance_officer.clone());

        // 2-of-3 quorum
        client.setup_multi_party_approval(&escrow_id, &sender, &approvers, &2, &0);

        // Sender approves
        let q1 = client.multi_party_approve(&escrow_id, &sender);
        assert_eq!(q1, false);

        // Compliance officer approves -> quorum met
        let q2 = client.multi_party_approve(&escrow_id, &compliance_officer);
        assert_eq!(q2, true);

        // Approve and release
        client.approve_escrow(&escrow_id, &admin);
        let recipient_balance_before = token.balance(&recipient);
        client.release_escrow(&escrow_id, &recipient, &token_addr);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Released);

        let recipient_balance_after = token.balance(&recipient);
        assert_eq!(recipient_balance_after - recipient_balance_before, 5000);

        // Config is finalized
        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.finalized, true);
    }

    #[test]
    fn test_multi_party_revoke_then_reapprove() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());
        approvers.push_back(admin.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        // Revoke sender's approval
        client.revoke_approval(&escrow_id, &sender);
        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 1);

        // Sender can re-approve
        let result = client.multi_party_approve(&escrow_id, &sender);
        assert_eq!(result, true);

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.approvals.len(), 2);
    }

    #[test]
    fn test_multi_party_setup_unauthorized() {
        let env = Env::default();
        let (client, _admin, sender, recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let unauthorized = Address::generate(&env);
        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        let result =
            client.try_setup_multi_party_approval(&escrow_id, &unauthorized, &approvers, &2, &5000);
        assert_eq!(result, Err(Ok(Error::Unauthorized)));
    }

    #[test]
    fn test_release_revoke_breaks_quorum() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);
        client.approve_escrow(&escrow_id, &admin);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        // Revoke one, breaking quorum
        client.revoke_approval(&escrow_id, &sender);

        let result = client.try_release_escrow(&escrow_id, &recipient, &token_addr);
        assert_eq!(result, Err(Ok(Error::QuorumNotMet)));

        // Re-approve to restore quorum
        client.multi_party_approve(&escrow_id, &sender);

        client.release_escrow(&escrow_id, &recipient, &token_addr);
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Released);
    }

    #[test]
    fn test_get_multi_party_status_none() {
        let env = Env::default();
        let (client, _admin, _sender, _recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_multi_party(&env);

        let status = client.get_multi_party_status(&escrow_id);
        assert!(status.is_none());
    }

    #[test]
    fn test_refund_finalized_after_quorum() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, _token, token_addr) =
            setup_escrow_for_multi_party(&env);

        let mut approvers = Vec::new(&env);
        approvers.push_back(sender.clone());
        approvers.push_back(recipient.clone());

        client.setup_multi_party_approval(&escrow_id, &admin, &approvers, &2, &5000);

        client.multi_party_approve(&escrow_id, &sender);
        client.multi_party_approve(&escrow_id, &recipient);

        client.refund_escrow(
            &escrow_id,
            &sender,
            &token_addr,
            &RefundReason::SenderRequest,
        );

        let config = client.get_multi_party_status(&escrow_id).unwrap();
        assert_eq!(config.finalized, true);
    }
    fn setup_escrow_for_dispute(
        env: &Env,
    ) -> (
        PaymentEscrowContractClient,
        Address,
        Address,
        Address,
        u64,
        token::Client,
        Address,
    ) {
        env.mock_all_auths();
        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(env, &contract_id);

        let admin = Address::generate(env);
        let sender = Address::generate(env);
        let recipient = Address::generate(env);

        let (token, token_admin) = create_token_contract(env, &admin);
        token_admin.mint(&sender, &5000);

        client.init_escrow(&admin);

        let asset = Asset {
            code: String::from_str(env, "USDC"),
            issuer: admin.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(env, "Dispute test"),
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        let token_addr = token.address.clone();
        (
            client, admin, sender, recipient, escrow_id, token, token_addr,
        )
    }

    #[test]
    fn test_raise_dispute() {
        let env = Env::default();
        let (client, _admin, sender, _recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_dispute(&env);

        let evidence_hash = BytesN::from_array(&env, &[0u8; 32]);
        client.raise_dispute(
            &escrow_id,
            &sender,
            &DisputeReason::NonDelivery,
            &evidence_hash,
        );

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Disputed);

        let dispute_opt = client.get_dispute(&escrow_id);
        assert!(dispute_opt.is_some());

        let dispute = dispute_opt.unwrap();
        assert_eq!(dispute.disputer, sender);
        assert_eq!(dispute.reason, DisputeReason::NonDelivery);
        assert_eq!(dispute.status, DisputeStatus::Open);
    }

    #[test]
    fn test_vote_and_resolve_favor_sender() {
        let env = Env::default();
        let (client, admin, sender, _recipient, escrow_id, token, token_addr) =
            setup_escrow_for_dispute(&env);

        let evidence_hash = BytesN::from_array(&env, &[0u8; 32]);
        client.raise_dispute(
            &escrow_id,
            &sender,
            &DisputeReason::NonDelivery,
            &evidence_hash,
        );

        // Admin votes favor sender
        client.vote_on_dispute(&escrow_id, &admin, &ResolutionOutcome::FavorSender);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Funded); // Should be back to Funded for refund

        let dispute = client.get_dispute(&escrow_id).unwrap();
        assert_eq!(dispute.status, DisputeStatus::Resolved);

        // Now refund should be possible
        let sender_balance_before = token.balance(&sender);
        client.refund_escrow(&escrow_id, &sender, &token_addr, &RefundReason::Dispute);

        let sender_balance_after = token.balance(&sender);
        assert_eq!(sender_balance_after - sender_balance_before, 1000);
    }

    #[test]
    fn test_vote_and_resolve_favor_recipient() {
        let env = Env::default();
        let (client, admin, sender, recipient, escrow_id, token, token_addr) =
            setup_escrow_for_dispute(&env);

        let evidence_hash = BytesN::from_array(&env, &[0u8; 32]);
        client.raise_dispute(
            &escrow_id,
            &sender,
            &DisputeReason::NonDelivery,
            &evidence_hash,
        );

        // Admin votes favor recipient
        client.vote_on_dispute(&escrow_id, &admin, &ResolutionOutcome::FavorRecipient);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Approved); // Should be Approved for release

        let dispute = client.get_dispute(&escrow_id).unwrap();
        assert_eq!(dispute.status, DisputeStatus::Resolved);

        // Now release should be possible
        let recipient_balance_before = token.balance(&recipient);
        client.release_escrow(&escrow_id, &recipient, &token_addr);

        let recipient_balance_after = token.balance(&recipient);
        assert_eq!(recipient_balance_after - recipient_balance_before, 1000);
    }

    #[test]
    fn test_unauthorized_raise_dispute() {
        let env = Env::default();
        let (client, _admin, _sender, _recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_dispute(&env);

        let unauthorized = Address::generate(&env);
        let evidence_hash = BytesN::from_array(&env, &[0u8; 32]);
        let result = client.try_raise_dispute(
            &escrow_id,
            &unauthorized,
            &DisputeReason::Fraud,
            &evidence_hash,
        );

        assert_eq!(result, Err(Ok(Error::UnauthorizedCaller)));
    }

    #[test]
    fn test_double_dispute_rejected() {
        let env = Env::default();
        let (client, _admin, sender, _recipient, escrow_id, _token, _token_addr) =
            setup_escrow_for_dispute(&env);

        let evidence_hash = BytesN::from_array(&env, &[0u8; 32]);
        client.raise_dispute(
            &escrow_id,
            &sender,
            &DisputeReason::NonDelivery,
            &evidence_hash,
        );

        let result =
            client.try_raise_dispute(&escrow_id, &sender, &DisputeReason::Fraud, &evidence_hash);
        assert_eq!(result, Err(Ok(Error::AlreadyDisputed)));
    }
}
