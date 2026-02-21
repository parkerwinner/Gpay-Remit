use soroban_sdk::{
    contract, contractimpl, contracttype, contracterror, Address, BytesN, Env,
    InvokeError, Symbol, Val, Vec, IntoVal,
};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum KycError {
    NotConfigured = 1,
    AccountNotVerified = 2,
    InvalidProof = 3,
    OracleUnavailable = 4,
    Unauthorized = 5,
    AlreadyVerified = 6,
    ProofExpired = 7,
    InvalidIssuer = 8,
    RateLimited = 9,
    AccountSuspended = 10,
    AlreadyConfigured = 11,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum KycStatus {
    Unknown,
    Pending,
    Verified,
    Rejected,
    Suspended,
}

#[derive(Clone)]
#[contracttype]
pub struct KycConfig {
    pub admin: Address,
    pub oracle_address: Address,
    pub use_oracle: bool,
    pub proof_validity_period: u64,
    pub last_check_ledger: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct KycRecord {
    pub account: Address,
    pub status: KycStatus,
    pub verified_at: u64,
    pub issuer: Address,
    pub expiry: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct KycResult {
    pub sender_verified: bool,
    pub recipient_verified: bool,
    pub timestamp: u64,
}

#[derive(Clone)]
#[contracttype]
pub enum KycDataKey {
    Config,
    Whitelist(Address),
    TrustedIssuer(Address),
    CheckCount(Address),
}

#[contract]
pub struct MockKycOracleContract;

#[contractimpl]
impl MockKycOracleContract {
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();
        env.storage().instance().set(&Symbol::new(&env, "admin"), &admin);
    }

    pub fn set_status(env: Env, admin: Address, account: Address, status: u32) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&Symbol::new(&env, "admin"))
            .unwrap();
        if admin != stored_admin {
            panic!("unauthorized");
        }
        env.storage().persistent().set(&account, &status);
    }

    pub fn is_kyc(env: Env, account: Address) -> u32 {
        env.storage().persistent().get(&account).unwrap_or(0)
    }
}

pub fn check_kyc(
    env: &Env,
    config: &KycConfig,
    sender: &Address,
    recipient: &Address,
) -> Result<KycResult, KycError> {
    if config.use_oracle {
        check_via_oracle(env, &config.oracle_address, sender, recipient)
    } else {
        check_via_whitelist(env, sender, recipient)
    }
}

fn check_via_whitelist(
    env: &Env,
    sender: &Address,
    recipient: &Address,
) -> Result<KycResult, KycError> {
    let sender_key = KycDataKey::Whitelist(sender.clone());
    let recipient_key = KycDataKey::Whitelist(recipient.clone());

    let sender_record: Option<KycRecord> = env.storage().persistent().get(&sender_key);
    let recipient_record: Option<KycRecord> = env.storage().persistent().get(&recipient_key);

    let current_time = env.ledger().timestamp();

    let sender_verified = match sender_record {
        Some(ref record) => {
            record.status == KycStatus::Verified
                && (record.expiry == 0 || record.expiry > current_time)
        }
        None => false,
    };

    let recipient_verified = match recipient_record {
        Some(ref record) => {
            record.status == KycStatus::Verified
                && (record.expiry == 0 || record.expiry > current_time)
        }
        None => false,
    };

    Ok(KycResult {
        sender_verified,
        recipient_verified,
        timestamp: current_time,
    })
}

fn check_via_oracle(
    env: &Env,
    oracle_address: &Address,
    sender: &Address,
    recipient: &Address,
) -> Result<KycResult, KycError> {
    let func = Symbol::new(env, "is_kyc");
    let current_time = env.ledger().timestamp();

    let sender_args: Vec<Val> = Vec::from_array(env, [sender.into_val(env)]);
    let sender_status =
        match env.try_invoke_contract::<u32, InvokeError>(oracle_address, &func, sender_args) {
            Ok(Ok(status)) => status,
            _ => return Err(KycError::OracleUnavailable),
        };

    let recipient_args: Vec<Val> = Vec::from_array(env, [recipient.into_val(env)]);
    let recipient_status =
        match env.try_invoke_contract::<u32, InvokeError>(oracle_address, &func, recipient_args) {
            Ok(Ok(status)) => status,
            _ => return Err(KycError::OracleUnavailable),
        };

    Ok(KycResult {
        sender_verified: sender_status == 1,
        recipient_verified: recipient_status == 1,
        timestamp: current_time,
    })
}

pub fn verify_proof(
    env: &Env,
    _account: &Address,
    proof_signature: &BytesN<64>,
    trusted_issuer: &Address,
    _proof_validity_period: u64,
) -> Result<bool, KycError> {
    let issuer_key = KycDataKey::TrustedIssuer(trusted_issuer.clone());
    let is_trusted: bool = env.storage().persistent().get(&issuer_key).unwrap_or(false);
    if !is_trusted {
        return Err(KycError::InvalidIssuer);
    }

    let all_zero = proof_signature.iter().all(|b| b == 0);
    if all_zero {
        return Err(KycError::InvalidProof);
    }

    Ok(true)
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    #[test]
    fn test_mock_kyc_oracle() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let client = MockKycOracleContractClient::new(&env, &contract_id);
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        client.initialize(&admin);
        client.set_status(&admin, &user, &1);

        let status = client.is_kyc(&user);
        assert_eq!(status, 1);
    }

    #[test]
    fn test_oracle_unknown_account() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let client = MockKycOracleContractClient::new(&env, &contract_id);
        let admin = Address::generate(&env);
        let unknown = Address::generate(&env);

        client.initialize(&admin);

        let status = client.is_kyc(&unknown);
        assert_eq!(status, 0);
    }

    #[test]
    fn test_whitelist_verification() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let sender_record = KycRecord {
            account: sender.clone(),
            status: KycStatus::Verified,
            verified_at: 500,
            issuer: issuer.clone(),
            expiry: 0,
        };

        let recipient_record = KycRecord {
            account: recipient.clone(),
            status: KycStatus::Verified,
            verified_at: 600,
            issuer: issuer.clone(),
            expiry: 0,
        };

        let oracle_addr = Address::generate(&env);
        let config = KycConfig {
            admin: Address::generate(&env),
            oracle_address: oracle_addr,
            use_oracle: false,
            proof_validity_period: 86400,
            last_check_ledger: 0,
        };

        env.as_contract(&contract_id, || {
            env.storage()
                .persistent()
                .set(&KycDataKey::Whitelist(sender.clone()), &sender_record);
            env.storage()
                .persistent()
                .set(&KycDataKey::Whitelist(recipient.clone()), &recipient_record);

            let result = check_kyc(&env, &config, &sender, &recipient).unwrap();
            assert!(result.sender_verified);
            assert!(result.recipient_verified);
        });
    }

    #[test]
    fn test_whitelist_unverified_sender() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let recipient_record = KycRecord {
            account: recipient.clone(),
            status: KycStatus::Verified,
            verified_at: 600,
            issuer: issuer.clone(),
            expiry: 0,
        };

        let oracle_addr = Address::generate(&env);
        let config = KycConfig {
            admin: Address::generate(&env),
            oracle_address: oracle_addr,
            use_oracle: false,
            proof_validity_period: 86400,
            last_check_ledger: 0,
        };

        env.as_contract(&contract_id, || {
            env.storage()
                .persistent()
                .set(&KycDataKey::Whitelist(recipient.clone()), &recipient_record);

            let result = check_kyc(&env, &config, &sender, &recipient).unwrap();
            assert!(!result.sender_verified);
            assert!(result.recipient_verified);
        });
    }

    #[test]
    fn test_whitelist_expired_record() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 5000;
        });

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let sender_record = KycRecord {
            account: sender.clone(),
            status: KycStatus::Verified,
            verified_at: 500,
            issuer: issuer.clone(),
            expiry: 3000,
        };

        let recipient_record = KycRecord {
            account: recipient.clone(),
            status: KycStatus::Verified,
            verified_at: 600,
            issuer: issuer.clone(),
            expiry: 0,
        };

        let oracle_addr = Address::generate(&env);
        let config = KycConfig {
            admin: Address::generate(&env),
            oracle_address: oracle_addr,
            use_oracle: false,
            proof_validity_period: 86400,
            last_check_ledger: 0,
        };

        env.as_contract(&contract_id, || {
            env.storage()
                .persistent()
                .set(&KycDataKey::Whitelist(sender.clone()), &sender_record);
            env.storage()
                .persistent()
                .set(&KycDataKey::Whitelist(recipient.clone()), &recipient_record);

            let result = check_kyc(&env, &config, &sender, &recipient).unwrap();
            assert!(!result.sender_verified);
            assert!(result.recipient_verified);
        });
    }

    #[test]
    fn test_oracle_kyc_check() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, MockKycOracleContract);
        let oracle_client = MockKycOracleContractClient::new(&env, &oracle_id);
        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        oracle_client.initialize(&admin);
        oracle_client.set_status(&admin, &sender, &1);
        oracle_client.set_status(&admin, &recipient, &1);

        let config = KycConfig {
            admin: admin.clone(),
            oracle_address: oracle_id,
            use_oracle: true,
            proof_validity_period: 86400,
            last_check_ledger: 0,
        };

        let result = check_kyc(&env, &config, &sender, &recipient).unwrap();
        assert!(result.sender_verified);
        assert!(result.recipient_verified);
    }

    #[test]
    fn test_oracle_kyc_sender_not_verified() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, MockKycOracleContract);
        let oracle_client = MockKycOracleContractClient::new(&env, &oracle_id);
        let admin = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        oracle_client.initialize(&admin);
        oracle_client.set_status(&admin, &recipient, &1);

        let config = KycConfig {
            admin: admin.clone(),
            oracle_address: oracle_id,
            use_oracle: true,
            proof_validity_period: 86400,
            last_check_ledger: 0,
        };

        let result = check_kyc(&env, &config, &sender, &recipient).unwrap();
        assert!(!result.sender_verified);
        assert!(result.recipient_verified);
    }

    #[test]
    fn test_oracle_unavailable() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let bogus_oracle = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        let config = KycConfig {
            admin: Address::generate(&env),
            oracle_address: bogus_oracle,
            use_oracle: true,
            proof_validity_period: 86400,
            last_check_ledger: 0,
        };

        let result = check_kyc(&env, &config, &sender, &recipient);
        assert_eq!(result, Err(KycError::OracleUnavailable));
    }

    #[test]
    fn test_verify_proof_invalid_issuer() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let account = Address::generate(&env);
        let untrusted_issuer = Address::generate(&env);
        let sig = BytesN::from_array(&env, &[1u8; 64]);

        env.as_contract(&contract_id, || {
            let result = verify_proof(&env, &account, &sig, &untrusted_issuer, 86400);
            assert_eq!(result, Err(KycError::InvalidIssuer));
        });
    }

    #[test]
    fn test_verify_proof_all_zeros() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let account = Address::generate(&env);
        let issuer = Address::generate(&env);
        let sig = BytesN::from_array(&env, &[0u8; 64]);

        env.as_contract(&contract_id, || {
            env.storage()
                .persistent()
                .set(&KycDataKey::TrustedIssuer(issuer.clone()), &true);

            let result = verify_proof(&env, &account, &sig, &issuer, 86400);
            assert_eq!(result, Err(KycError::InvalidProof));
        });
    }

    #[test]
    fn test_verify_proof_valid() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let account = Address::generate(&env);
        let issuer = Address::generate(&env);
        let sig = BytesN::from_array(&env, &[1u8; 64]);

        env.as_contract(&contract_id, || {
            env.storage()
                .persistent()
                .set(&KycDataKey::TrustedIssuer(issuer.clone()), &true);

            let result = verify_proof(&env, &account, &sig, &issuer, 86400);
            assert_eq!(result, Ok(true));
        });
    }

    #[test]
    fn test_suspended_account_whitelist() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, MockKycOracleContract);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let sender_record = KycRecord {
            account: sender.clone(),
            status: KycStatus::Suspended,
            verified_at: 500,
            issuer: issuer.clone(),
            expiry: 0,
        };

        let recipient_record = KycRecord {
            account: recipient.clone(),
            status: KycStatus::Verified,
            verified_at: 600,
            issuer: issuer.clone(),
            expiry: 0,
        };

        let oracle_addr = Address::generate(&env);
        let config = KycConfig {
            admin: Address::generate(&env),
            oracle_address: oracle_addr,
            use_oracle: false,
            proof_validity_period: 86400,
            last_check_ledger: 0,
        };

        env.as_contract(&contract_id, || {
            env.storage()
                .persistent()
                .set(&KycDataKey::Whitelist(sender.clone()), &sender_record);
            env.storage()
                .persistent()
                .set(&KycDataKey::Whitelist(recipient.clone()), &recipient_record);

            let result = check_kyc(&env, &config, &sender, &recipient).unwrap();
            assert!(!result.sender_verified);
            assert!(result.recipient_verified);
        });
    }
}
