use soroban_sdk::{contract, contractimpl, contracttype, contracterror, Address, Env, String, Vec, symbol_short};

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
}

#[derive(Clone, PartialEq, Eq, Debug)]
#[contracttype]
pub enum EscrowStatus {
    Pending,
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
    pub asset: Asset,
    pub release_conditions: ReleaseCondition,
    pub status: EscrowStatus,
    pub created_at: u64,
    pub escrow_id: u64,
}

#[derive(Clone, Copy)]
#[contracttype]
pub enum DataKey {
    EscrowCounter,
    Escrow(u64),
    Admin,
    SupportedAssets,
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

    pub fn create_escrow(
        env: Env,
        sender: Address,
        recipient: Address,
        amount: i128,
        asset: Asset,
        expiration_timestamp: u64,
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
            asset,
            release_conditions: ReleaseCondition {
                expiration_timestamp,
                recipient_approval: false,
                oracle_confirmation: false,
            },
            status: EscrowStatus::Pending,
            created_at: env.ledger().timestamp(),
            escrow_id: counter,
        };

        env.storage().instance().set(&DataKey::Escrow(counter), &escrow);
        env.storage().instance().set(&DataKey::EscrowCounter, &counter);

        env.events().publish((symbol_short!("created"), counter), escrow.sender);

        Ok(counter)
    }

    pub fn get_escrow(env: Env, escrow_id: u64) -> Option<Escrow> {
        env.storage().instance().get(&DataKey::Escrow(escrow_id))
    }

    pub fn approve_escrow(env: Env, escrow_id: u64, approver: Address) -> Result<(), Error> {
        approver.require_auth();

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if escrow.status != EscrowStatus::Pending {
            return Err(Error::InvalidStatus);
        }

        escrow.status = EscrowStatus::Approved;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("approved"), escrow_id), approver);

        Ok(())
    }

    pub fn release_escrow(env: Env, escrow_id: u64, caller: Address) -> Result<(), Error> {
        caller.require_auth();

        let mut escrow: Escrow = env.storage().instance().get(&DataKey::Escrow(escrow_id)).ok_or(Error::EscrowNotFound)?;

        if escrow.status != EscrowStatus::Approved {
            return Err(Error::NotApproved);
        }

        if env.ledger().timestamp() > escrow.release_conditions.expiration_timestamp {
            escrow.status = EscrowStatus::Expired;
            env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);
            return Err(Error::Expired);
        }

        escrow.status = EscrowStatus::Released;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("released"), escrow_id), caller);

        Ok(())
    }

    pub fn refund_escrow(env: Env, escrow_id: u64, caller: Address) -> Result<(), Error> {
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

        escrow.status = EscrowStatus::Refunded;
        env.storage().instance().set(&DataKey::Escrow(escrow_id), &escrow);

        env.events().publish((symbol_short!("refunded"), escrow_id), caller);

        Ok(())
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

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

        let escrow_id = client.create_escrow(&sender, &recipient, &1000, &asset, &2000);
        assert_eq!(escrow_id, 1);

        let escrow = client.get_escrow(&escrow_id);
        assert!(escrow.is_some());
        
        let escrow_data = escrow.unwrap();
        assert_eq!(escrow_data.amount, 1000);
        assert_eq!(escrow_data.sender, sender);
        assert_eq!(escrow_data.recipient, recipient);
        assert_eq!(escrow_data.status, EscrowStatus::Pending);
        assert_eq!(escrow_data.created_at, 1000);
    }

    #[test]
    fn test_invalid_amount() {
        let env = Env::default();
        env.mock_all_auths();

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

        let result = client.try_create_escrow(&sender, &recipient, &0, &asset, &2000);
        assert_eq!(result, Err(Ok(Error::InvalidAmount)));
    }

    #[test]
    fn test_same_sender_recipient() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let issuer = Address::generate(&env);

        client.initialize(&admin);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: issuer.clone(),
        };

        client.add_supported_asset(&admin, &asset);

        let result = client.try_create_escrow(&sender, &sender, &1000, &asset, &2000);
        assert_eq!(result, Err(Ok(Error::SameSenderRecipient)));
    }

    #[test]
    fn test_approve_and_release() {
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

        let escrow_id = client.create_escrow(&sender, &recipient, &1000, &asset, &2000);

        client.approve_escrow(&escrow_id, &admin);
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Approved);

        client.release_escrow(&escrow_id, &recipient);
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Released);
    }

    #[test]
    fn test_refund_after_expiration() {
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

        let escrow_id = client.create_escrow(&sender, &recipient, &1000, &asset, &2000);

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        client.refund_escrow(&escrow_id, &sender);
        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Refunded);
    }
}
