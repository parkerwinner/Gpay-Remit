use soroban_sdk::{contract, contractimpl, contracttype, contracterror, token, Address, Env, String, Vec, symbol_short};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum Error {
    InvalidAmount = 1,
    SameSenderRecipient = 2,
    UnsupportedAsset = 3,
    CounterOverflow = 4,
    EscrowNotFound = 5,
    InvalidStatus = 6,
    NotApproved = 7,
    Expired = 8,
    Unauthorized = 9,
    AlreadyReleased = 10,
    NotExpired = 11,
    WrongSender = 12,
    EscrowNotPending = 13,
    InvalidAsset = 14,
    InsufficientAmount = 15,
    AlreadyFunded = 16,
    DepositOverflow = 17,
    ConditionsNotMet = 18,
    UnauthorizedCaller = 19,
    InsufficientFunds = 20,
    ConversionFailed = 21,
    InvalidFeePercentage = 22,
    PartialReleaseNotAllowed = 23,
    ArithmeticOverflow = 24,
    AlreadyRefunded = 25,
    UnauthorizedRefund = 26,
    NoFundsAvailable = 27,
    InvalidRefundAmount = 28,
    SignatureMismatch = 29,
    OracleFailure = 30,
    InvalidProof = 31,
    TimestampNotReached = 32,
    ApprovalRequired = 33,
    OracleDataMissing = 34,
    FeeExceedsAmount = 35,
    InvalidRate = 36,
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

#[derive(Clone)]
#[contracttype]
pub struct FeeStructure {
    pub platform_percentage: i128,
    pub forex_percentage: i128,
    pub compliance_flat: i128,
    pub network_flat: i128,
    pub min_fee: i128,
    pub max_fee: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct Condition {
    pub condition_type: ConditionType,
    pub required: bool,
    pub verified: bool,
    pub threshold_value: i128,
}

#[derive(Clone)]
#[contracttype]
pub struct VerificationResult {
    pub all_passed: bool,
    pub failed_conditions: Vec<ConditionType>,
}

#[derive(Clone)]
#[contracttype]
pub struct Asset {
    pub code: String,
    pub issuer: Address,
}

#[derive(Clone)]
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

#[derive(Clone)]
#[contracttype]
pub struct Escrow {
    pub sender: Address,
    pub recipient: Address,
    pub amount: i128,
    pub deposited_amount: i128,
    pub released_amount: i128,
    pub refunded_amount: i128,
    pub asset: Asset,
    pub release_conditions: ReleaseCondition,
    pub status: EscrowStatus,
    pub created_at: u64,
    pub last_deposit_at: u64,
    pub release_timestamp: u64,
    pub refund_timestamp: u64,
    pub escrow_id: u64,
    pub memo: String,
    pub allow_partial_release: bool,
}

#[derive(Clone, Copy)]
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
}

#[contract]
pub struct PaymentEscrowContract;

#[contractimpl]
impl PaymentEscrowContract {
    pub fn initialize(env: Env, admin: Address) {
        if env.storage().instance().has(&DataKey::Admin) {
            panic!("Already initialized");
        }
        
        env.storage().instance().set(&DataKey::Admin, &admin);
        env.storage().instance().set(&DataKey::EscrowCounter, &0u64);
        env.storage().instance().set(&DataKey::SupportedAssets, &Vec::<Asset>::new(&env));
        env.storage().instance().set(&DataKey::PlatformFeePercentage, &0i128);
        env.storage().instance().set(&DataKey::ProcessingFeePercentage, &0i128);
        env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
    }

    pub fn add_supported_asset(env: Env, admin: Address, asset: Asset) {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            panic!("Unauthorized");
        }

        let mut assets: Vec<Asset> = env.storage().instance().get(&DataKey::SupportedAssets).unwrap();
        assets.push_back(asset);
        env.storage().instance().set(&DataKey::SupportedAssets, &assets);
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

        env.storage().instance().set(&DataKey::PlatformFeePercentage, &fee_percentage);
        
        env.events().publish((symbol_short!("fee_set"),), fee_percentage);
        
        Ok(())
    }

    pub fn get_platform_fee(env: Env) -> i128 {
        env.storage().instance().get(&DataKey::PlatformFeePercentage).unwrap_or(0)
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

        env.storage().instance().set(&DataKey::ProcessingFeePercentage, &fee_percentage);
        
        env.events().publish((symbol_short!("proc_fee"),), fee_percentage);
        
        Ok(())
    }

    pub fn get_processing_fee(env: Env) -> i128 {
        env.storage().instance().get(&DataKey::ProcessingFeePercentage).unwrap_or(0)
    }

    pub fn set_fee_wallet(env: Env, admin: Address, fee_wallet: Address) -> Result<(), Error> {
        admin.require_auth();
        
        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if admin != stored_admin {
            return Err(Error::Unauthorized);
        }

        env.storage().instance().set(&DataKey::FeeWallet, &fee_wallet);
        
        env.events().publish((symbol_short!("fee_wal"),), fee_wallet);
        
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
            return Err(Error::InvalidRate);
        }

        env.storage().instance().set(&DataKey::ForexFeePercentage, &fee_percentage);
        
        env.events().publish((symbol_short!("forex_f"),), fee_percentage);
        
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

        env.storage().instance().set(&DataKey::ComplianceFlatFee, &flat_fee);
        
        env.events().publish((symbol_short!("comp_fee"),), flat_fee);
        
        Ok(())
    }

    pub fn set_fee_limits(env: Env, admin: Address, min_fee: i128, max_fee: i128) -> Result<(), Error> {
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
        
        env.events().publish((symbol_short!("fee_lim"),), (min_fee, max_fee));
        
        Ok(())
    }

    fn calculate_fees(env: &Env, amount: i128) -> Result<FeeBreakdown, Error> {
        let platform_percentage = env.storage().instance().get(&DataKey::PlatformFeePercentage).unwrap_or(0);
        let forex_percentage = env.storage().instance().get(&DataKey::ForexFeePercentage).unwrap_or(0);
        let compliance_flat = env.storage().instance().get(&DataKey::ComplianceFlatFee).unwrap_or(0);
        let network_flat = env.storage().instance().get(&DataKey::NetworkFlatFee).unwrap_or(0);

        let platform_fee = amount.checked_mul(platform_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let forex_fee = amount.checked_mul(forex_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let mut total_fee = platform_fee.checked_add(forex_fee)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_add(compliance_flat)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_add(network_flat)
            .ok_or(Error::ArithmeticOverflow)?;

        let min_fee = env.storage().instance().get(&DataKey::MinFee).unwrap_or(0);
        let max_fee = env.storage().instance().get(&DataKey::MaxFee).unwrap_or(i128::MAX);

        if total_fee < min_fee {
            total_fee = min_fee;
        }
        if total_fee > max_fee {
            total_fee = max_fee;
        }

        if total_fee >= amount {
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

    pub fn create_escrow(
        env: Env,
        sender: Address,
        recipient: Address,
        amount: i128,
        asset: Asset,
        expiration_timestamp: u64,
        memo: String,
    ) -> Result<u64, Error> {
        sender.require_auth();

        if amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        if sender == recipient {
            return Err(Error::SameSenderRecipient);
        }

        let assets: Vec<Asset> = env.storage().instance().get(&DataKey::SupportedAssets).unwrap();
        let mut asset_supported = false;
        for supported_asset in assets.iter() {
            if supported_asset.code == asset.code && supported_asset.issuer == asset.issuer {
                asset_supported = true;
                break;
            }
        }
        
        if !asset_supported {
            return Err(Error::UnsupportedAsset);
        }

        let mut counter: u64 = env.storage().instance().get(&DataKey::EscrowCounter).unwrap_or(0);
        counter = counter.checked_add(1).ok_or(Error::CounterOverflow)?;

        let escrow = Escrow {
            sender: sender.clone(),
            recipient,
            amount,
            deposited_amount: 0,
            released_amount: 0,
            refunded_amount: 0,
            asset,
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
        };

        env.storage().instance().set(&DataKey::Escrow(counter), &escrow);
        env.storage().instance().set(&DataKey::EscrowCounter, &counter);

        env.events().publish((symbol_short!("created"), counter), escrow.sender);

        Ok(counter)
    }

    pub fn deposit(
        env: Env,
        escrow_id: u64,
        caller: Address,
        amount: i128,
        token_address: Address,
    ) -> Result<(), Error> {
        caller.require_auth();

        if amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if caller != escrow.sender {
            return Err(Error::WrongSender);
        }

        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded {
            return Err(Error::EscrowNotPending);
        }

        let new_deposited = escrow.deposited_amount.checked_add(amount).ok_or(Error::DepositOverflow)?;

        if new_deposited > escrow.amount {
            return Err(Error::InsufficientAmount);
        }

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();
        
        token_client.transfer(&caller, &contract_address, &amount);

        escrow.deposited_amount = new_deposited;
        escrow.last_deposit_at = env.ledger().timestamp();

        if escrow.deposited_amount == escrow.amount {
            escrow.status = EscrowStatus::Funded;
        }

        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish(
            (symbol_short!("deposit"), escrow_id),
            (caller, amount, escrow.deposited_amount)
        );

        Ok(())
    }

    pub fn get_escrow(env: Env, escrow_id: u64) -> Option<Escrow> {
        env.storage().instance().get(&DataKey::Escrow(escrow_id))
    }

    pub fn approve_escrow(env: Env, escrow_id: u64, approver: Address) -> Result<(), Error> {
        approver.require_auth();

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if escrow.status != EscrowStatus::Funded {
            return Err(Error::InvalidStatus);
        }

        escrow.status = EscrowStatus::Approved;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("approved"), escrow_id), approver);

        Ok(())
    }

    pub fn release_escrow(env: Env, escrow_id: u64, caller: Address, token_address: Address) -> Result<(), Error> {
        caller.require_auth();

        let guard: bool = env.storage().instance().get(&DataKey::ReentrancyGuard).unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage().instance().set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if escrow.status != EscrowStatus::Approved && escrow.status != EscrowStatus::Funded {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::NotApproved);
        }

        if escrow.status == EscrowStatus::Released && !escrow.allow_partial_release {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyReleased);
        }

        let current_time = env.ledger().timestamp();
        if current_time > escrow.release_conditions.expiration_timestamp {
            escrow.status = EscrowStatus::Expired;
            env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::Expired);
        }

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.recipient && caller != stored_admin && caller != escrow.sender {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::UnauthorizedCaller);
        }

        if escrow.deposited_amount == 0 {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let available_amount = escrow.deposited_amount.checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if available_amount <= 0 {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let fee_percentage = Self::get_platform_fee(env.clone());
        let fee_amount = available_amount.checked_mul(fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let recipient_amount = available_amount.checked_sub(fee_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if recipient_amount <= 0 {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientAmount);
        }

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();
        
        token_client.transfer(&contract_address, &escrow.recipient, &recipient_amount);

        if fee_amount > 0 {
            token_client.transfer(&contract_address, &stored_admin, &fee_amount);
        }

        escrow.released_amount = escrow.released_amount.checked_add(available_amount)
            .ok_or(Error::ArithmeticOverflow)?;
        escrow.status = EscrowStatus::Released;
        escrow.release_timestamp = current_time;
        
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish(
            (symbol_short!("released"), escrow_id),
            (caller.clone(), recipient_amount, fee_amount, current_time)
        );

        env.storage().instance().set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn release_partial(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
        release_amount: i128,
    ) -> Result<(), Error> {
        caller.require_auth();

        if release_amount <= 0 {
            return Err(Error::InvalidAmount);
        }

        let guard: bool = env.storage().instance().get(&DataKey::ReentrancyGuard).unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage().instance().set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if !escrow.allow_partial_release {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::PartialReleaseNotAllowed);
        }

        if escrow.status != EscrowStatus::Approved && escrow.status != EscrowStatus::Funded && escrow.status != EscrowStatus::Released {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InvalidStatus);
        }

        let current_time = env.ledger().timestamp();
        if current_time > escrow.release_conditions.expiration_timestamp {
            escrow.status = EscrowStatus::Expired;
            env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::Expired);
        }

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.recipient && caller != stored_admin {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::UnauthorizedCaller);
        }

        let available_amount = escrow.deposited_amount.checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if release_amount > available_amount {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let fee_percentage = Self::get_platform_fee(env.clone());
        let fee_amount = release_amount.checked_mul(fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let recipient_amount = release_amount.checked_sub(fee_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();
        
        token_client.transfer(&contract_address, &escrow.recipient, &recipient_amount);

        if fee_amount > 0 {
            token_client.transfer(&contract_address, &stored_admin, &fee_amount);
        }

        escrow.released_amount = escrow.released_amount.checked_add(release_amount)
            .ok_or(Error::ArithmeticOverflow)?;
        
        if escrow.released_amount >= escrow.deposited_amount {
            escrow.status = EscrowStatus::Released;
        }
        
        escrow.release_timestamp = current_time;
        
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish(
            (symbol_short!("partial"), escrow_id),
            (caller.clone(), recipient_amount, fee_amount, escrow.released_amount)
        );

        env.storage().instance().set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn enable_partial_release(env: Env, escrow_id: u64, caller: Address) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if caller != escrow.sender {
            return Err(Error::Unauthorized);
        }

        escrow.allow_partial_release = true;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("part_enab"), escrow_id), caller);

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

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

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
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("cond_add"), escrow_id), condition_type);

        Ok(())
    }

    pub fn set_condition_operator(
        env: Env,
        escrow_id: u64,
        caller: Address,
        operator: ConditionOperator,
    ) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        escrow.release_conditions.operator = operator;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("cond_op"), escrow_id), operator);

        Ok(())
    }

    pub fn verify_conditions(
        env: Env,
        escrow_id: u64,
        proof_data: i128,
    ) -> Result<VerificationResult, Error> {
        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

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
                },
                ConditionType::Approval => {
                    escrow.release_conditions.current_approvals >= escrow.release_conditions.min_approvals
                },
                ConditionType::OraclePrice => {
                    if proof_data > 0 {
                        proof_data >= condition.threshold_value
                    } else {
                        false
                    }
                },
                ConditionType::MultiSignature => {
                    escrow.release_conditions.current_approvals >= escrow.release_conditions.min_approvals
                },
                ConditionType::KYCVerified => {
                    escrow.release_conditions.recipient_approval
                },
            };

            condition.verified = verified;
            escrow.release_conditions.conditions.set(i, condition);

            if verified {
                passed_count += 1;
            } else if is_required {
                failed_conditions.push_back(condition_type_copy);
            }
        }

        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        let all_passed = match escrow.release_conditions.operator {
            ConditionOperator::And => {
                failed_conditions.is_empty() && (required_count == 0 || passed_count >= required_count)
            },
            ConditionOperator::Or => {
                passed_count > 0
            },
        };

        let result = VerificationResult {
            all_passed,
            failed_conditions,
        };

        env.events().publish(
            (symbol_short!("verified"), escrow_id),
            (all_passed, passed_count)
        );

        Ok(result)
    }

    pub fn add_approval(
        env: Env,
        escrow_id: u64,
        approver: Address,
    ) -> Result<(), Error> {
        approver.require_auth();

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if approver != stored_admin && approver != escrow.recipient && approver != escrow.sender {
            return Err(Error::Unauthorized);
        }

        escrow.release_conditions.current_approvals = escrow.release_conditions.current_approvals.checked_add(1)
            .unwrap_or(escrow.release_conditions.current_approvals);

        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish(
            (symbol_short!("approval"), escrow_id),
            (approver, escrow.release_conditions.current_approvals)
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

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            return Err(Error::Unauthorized);
        }

        escrow.release_conditions.min_approvals = min_approvals;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("min_appr"), escrow_id), min_approvals);

        Ok(())
    }

    pub fn refund_escrow(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
        reason: RefundReason,
    ) -> Result<(), Error> {
        caller.require_auth();

        let guard: bool = env.storage().instance().get(&DataKey::ReentrancyGuard).unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage().instance().set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::UnauthorizedRefund);
        }

        if escrow.status == EscrowStatus::Released {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyReleased);
        }

        if escrow.status == EscrowStatus::Refunded {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyRefunded);
        }

        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded && escrow.status != EscrowStatus::Approved {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InvalidStatus);
        }

        let current_time = env.ledger().timestamp();
        
        if reason == RefundReason::Expiration {
            if current_time <= escrow.release_conditions.expiration_timestamp {
                env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
                return Err(Error::NotExpired);
            }
        }

        let available_for_refund = escrow.deposited_amount.checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_sub(escrow.refunded_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if available_for_refund <= 0 {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::NoFundsAvailable);
        }

        let processing_fee_percentage = Self::get_processing_fee(env.clone());
        let processing_fee = available_for_refund.checked_mul(processing_fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let refund_amount = available_for_refund.checked_sub(processing_fee)
            .ok_or(Error::ArithmeticOverflow)?;

        if refund_amount > 0 {
            let token_client = token::Client::new(&env, &token_address);
            let contract_address = env.current_contract_address();
            
            token_client.transfer(&contract_address, &escrow.sender, &refund_amount);

            if processing_fee > 0 {
                token_client.transfer(&contract_address, &stored_admin, &processing_fee);
            }
        }

        escrow.refunded_amount = escrow.refunded_amount.checked_add(available_for_refund)
            .ok_or(Error::ArithmeticOverflow)?;
        escrow.status = EscrowStatus::Refunded;
        escrow.refund_timestamp = current_time;
        
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish(
            (symbol_short!("refunded"), escrow_id),
            (caller.clone(), refund_amount, processing_fee, reason)
        );

        env.storage().instance().set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }

    pub fn refund_partial(
        env: Env,
        escrow_id: u64,
        caller: Address,
        token_address: Address,
        refund_amount: i128,
        reason: RefundReason,
    ) -> Result<(), Error> {
        caller.require_auth();

        if refund_amount <= 0 {
            return Err(Error::InvalidRefundAmount);
        }

        let guard: bool = env.storage().instance().get(&DataKey::ReentrancyGuard).unwrap_or(false);
        if guard {
            return Err(Error::UnauthorizedCaller);
        }
        env.storage().instance().set(&DataKey::ReentrancyGuard, &true);

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        let stored_admin: Address = env.storage().instance().get(&DataKey::Admin).unwrap();
        if caller != escrow.sender && caller != stored_admin {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::UnauthorizedRefund);
        }

        if escrow.status == EscrowStatus::Released {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::AlreadyReleased);
        }

        if escrow.status != EscrowStatus::Pending && escrow.status != EscrowStatus::Funded && escrow.status != EscrowStatus::Approved && escrow.status != EscrowStatus::Refunded {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InvalidStatus);
        }

        let available_for_refund = escrow.deposited_amount.checked_sub(escrow.released_amount)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_sub(escrow.refunded_amount)
            .ok_or(Error::ArithmeticOverflow)?;

        if refund_amount > available_for_refund {
            env.storage().instance().set(&DataKey::ReentrancyGuard, &false);
            return Err(Error::InsufficientFunds);
        }

        let processing_fee_percentage = Self::get_processing_fee(env.clone());
        let processing_fee = refund_amount.checked_mul(processing_fee_percentage)
            .ok_or(Error::ArithmeticOverflow)?
            .checked_div(10000)
            .ok_or(Error::ArithmeticOverflow)?;

        let net_refund = refund_amount.checked_sub(processing_fee)
            .ok_or(Error::ArithmeticOverflow)?;

        let token_client = token::Client::new(&env, &token_address);
        let contract_address = env.current_contract_address();
        
        token_client.transfer(&contract_address, &escrow.sender, &net_refund);

        if processing_fee > 0 {
            token_client.transfer(&contract_address, &stored_admin, &processing_fee);
        }

        escrow.refunded_amount = escrow.refunded_amount.checked_add(refund_amount)
            .ok_or(Error::ArithmeticOverflow)?;
        
        let current_time = env.ledger().timestamp();
        escrow.refund_timestamp = current_time;

        let total_processed = escrow.released_amount.checked_add(escrow.refunded_amount)
            .ok_or(Error::ArithmeticOverflow)?;
        
        if total_processed >= escrow.deposited_amount {
            escrow.status = EscrowStatus::Refunded;
        }
        
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish(
            (symbol_short!("ref_part"), escrow_id),
            (caller.clone(), net_refund, processing_fee, escrow.refunded_amount)
        );

        env.storage().instance().set(&DataKey::ReentrancyGuard, &false);

        Ok(())
    }
}


#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::{testutils::{Address as _, Ledger}, token};

    fn create_token_contract<'a>(env: &Env, admin: &Address) -> (token::Client<'a>, token::StellarAssetClient<'a>) {
        let contract_address = env.register_stellar_asset_contract_v2(admin.clone());
        (
            token::Client::new(env, &contract_address.address()),
            token::StellarAssetClient::new(env, &contract_address.address()),
        )
    }

    #[test]
    fn test_initialize() {
        let env = Env::default();
        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        env.mock_all_auths();
        
        client.initialize(&admin);
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Payment for services")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test payment")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test payment")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let sender_balance_before = token.balance(&sender);
        client.refund_escrow(&escrow_id, &sender, &token.address, &RefundReason::Expiration);
        
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
        client.initialize(&admin);

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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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
        assert_eq!(recipient_balance_after - recipient_balance_before, 1000 - fee);
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);
        client.approve_escrow(&escrow_id, &admin);

        let result = client.try_release_escrow(&escrow_id, &unauthorized, &token.address);
        assert_eq!(result, Err(Ok(Error::UnauthorizedCaller)));
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let sender_balance_before = token.balance(&sender);
        let admin_balance_before = token.balance(&admin);
        
        client.refund_escrow(&escrow_id, &sender, &token.address, &RefundReason::Expiration);
        
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        let sender_balance_before = token.balance(&sender);
        client.refund_escrow(&escrow_id, &admin, &token.address, &RefundReason::AdminAction);
        
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        let sender_balance_before = token.balance(&sender);
        
        client.refund_partial(&escrow_id, &sender, &token.address, &400, &RefundReason::Dispute);
        
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.refunded_amount, 400);
        
        let sender_balance_after = token.balance(&sender);
        assert_eq!(sender_balance_after - sender_balance_before, 400);

        client.refund_partial(&escrow_id, &sender, &token.address, &600, &RefundReason::Dispute);
        
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        let result = client.try_refund_escrow(&escrow_id, &unauthorized, &token.address, &RefundReason::Expiration);
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.deposit(&escrow_id, &sender, &1000, &token.address);
        client.approve_escrow(&escrow_id, &admin);
        client.release_escrow(&escrow_id, &recipient, &token.address);

        let result = client.try_refund_escrow(&escrow_id, &sender, &token.address, &RefundReason::Expiration);
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::OraclePrice, &true, &100);

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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::OraclePrice, &true, &100);

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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::Timestamp, &true, &0);
        client.add_condition(&escrow_id, &sender, &ConditionType::OraclePrice, &true, &100);
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
        );

        client.add_condition(&escrow_id, &sender, &ConditionType::Timestamp, &true, &0);
        client.add_condition(&escrow_id, &sender, &ConditionType::OraclePrice, &true, &100);
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

        client.initialize(&admin);

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
            &String::from_str(&env, "Test")
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
        client.initialize(&admin);

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
        client.initialize(&admin);

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

        client.initialize(&admin);
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
        client.initialize(&admin);

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
        client.initialize(&admin);

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
        client.initialize(&admin);

        client.set_compliance_fee(&admin, &25);

        let breakdown = client.get_fee_breakdown(&1000);
        assert_eq!(breakdown.compliance_fee, 25);
    }
}
