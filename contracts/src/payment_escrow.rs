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
}

#[derive(Clone)]
#[contracttype]
pub struct Escrow {
    pub sender: Address,
    pub recipient: Address,
    pub amount: i128,
    pub deposited_amount: i128,
    pub released_amount: i128,
    pub asset: Asset,
    pub release_conditions: ReleaseCondition,
    pub status: EscrowStatus,
    pub created_at: u64,
    pub last_deposit_at: u64,
    pub release_timestamp: u64,
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
            asset,
            release_conditions: ReleaseCondition {
                expiration_timestamp,
                recipient_approval: false,
                oracle_confirmation: false,
            },
            status: EscrowStatus::Pending,
            created_at: env.ledger().timestamp(),
            last_deposit_at: 0,
            release_timestamp: 0,
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

    pub fn refund_escrow(env: Env, escrow_id: u64, caller: Address, token_address: Address) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if escrow.sender != caller {
            return Err(Error::Unauthorized);
        }

        if escrow.status == EscrowStatus::Released {
            return Err(Error::AlreadyReleased);
        }

        if env.ledger().timestamp() <= escrow.release_conditions.expiration_timestamp {
            return Err(Error::NotExpired);
        }

        if escrow.deposited_amount > 0 {
            let token_client = token::Client::new(&env, &token_address);
            let contract_address = env.current_contract_address();
            
            token_client.transfer(&contract_address, &escrow.sender, &escrow.deposited_amount);
        }

        escrow.status = EscrowStatus::Refunded;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("refunded"), escrow_id), caller);

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
        client.refund_escrow(&escrow_id, &sender, &token.address);
        
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Refunded);

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
}
