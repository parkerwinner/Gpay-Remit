use soroban_sdk::{
    contract, contracterror, contractimpl, contracttype, Address, Env, IntoVal, InvokeError,
    Symbol, Val, Vec,
};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum AmlError {
    NotConfigured = 1,
    OracleUnavailable = 2,
    HighRisk = 3,
    Unauthorized = 4,
    InvalidThreshold = 5,
    FlagNotFound = 6,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum AmlStatus {
    Clear,
    Flagged,
    Reviewing,
    Cleared,
}

#[derive(Clone)]
#[contracttype]
pub struct AmlConfig {
    pub admin: Address,
    pub oracle_address: Address,
    pub risk_threshold: u32,
    pub enabled: bool,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct AmlScreeningResult {
    pub sender: Address,
    pub recipient: Address,
    pub amount: i128,
    pub risk_score: u32,
    pub status: AmlStatus,
    pub timestamp: u64,
}

#[contract]
pub struct MockAmlOracleContract;

#[contractimpl]
impl MockAmlOracleContract {
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();
        env.storage()
            .instance()
            .set(&Symbol::new(&env, "admin"), &admin);
    }

    pub fn set_risk_score(env: Env, admin: Address, account: Address, score: u32) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&Symbol::new(&env, "admin"))
            .unwrap();
        if admin != stored_admin {
            panic!("unauthorized");
        }
        env.storage().persistent().set(&account, &score);
    }

    pub fn screen(env: Env, sender: Address, recipient: Address, _amount: i128) -> u32 {
        let sender_score: u32 = env.storage().persistent().get(&sender).unwrap_or(0);
        let recipient_score: u32 = env.storage().persistent().get(&recipient).unwrap_or(0);
        if sender_score > recipient_score {
            sender_score
        } else {
            recipient_score
        }
    }
}

pub fn screen_transaction(
    env: &Env,
    config: &AmlConfig,
    sender: &Address,
    recipient: &Address,
    amount: i128,
) -> Result<AmlScreeningResult, AmlError> {
    if !config.enabled {
        return Ok(AmlScreeningResult {
            sender: sender.clone(),
            recipient: recipient.clone(),
            amount,
            risk_score: 0,
            status: AmlStatus::Clear,
            timestamp: env.ledger().timestamp(),
        });
    }

    let risk_score = query_aml_oracle(env, &config.oracle_address, sender, recipient, amount)?;

    let status = if risk_score > config.risk_threshold {
        AmlStatus::Flagged
    } else {
        AmlStatus::Clear
    };

    Ok(AmlScreeningResult {
        sender: sender.clone(),
        recipient: recipient.clone(),
        amount,
        risk_score,
        status,
        timestamp: env.ledger().timestamp(),
    })
}

fn query_aml_oracle(
    env: &Env,
    oracle_address: &Address,
    sender: &Address,
    recipient: &Address,
    amount: i128,
) -> Result<u32, AmlError> {
    let func = Symbol::new(env, "screen");
    let args: Vec<Val> = Vec::from_array(
        env,
        [
            sender.into_val(env),
            recipient.into_val(env),
            amount.into_val(env),
        ],
    );

    match env.try_invoke_contract::<u32, InvokeError>(oracle_address, &func, args) {
        Ok(Ok(score)) => Ok(score),
        _ => Err(AmlError::OracleUnavailable),
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    #[test]
    fn test_mock_aml_oracle() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, MockAmlOracleContract);
        let client = MockAmlOracleContractClient::new(&env, &contract_id);
        let admin = Address::generate(&env);
        let user = Address::generate(&env);

        client.initialize(&admin);
        client.set_risk_score(&admin, &user, &75);

        let other = Address::generate(&env);
        let score = client.screen(&user, &other, &1000);
        assert_eq!(score, 75);
    }

    #[test]
    fn test_mock_aml_oracle_unknown_account() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, MockAmlOracleContract);
        let client = MockAmlOracleContractClient::new(&env, &contract_id);
        let admin = Address::generate(&env);

        client.initialize(&admin);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let score = client.screen(&sender, &recipient, &1000);
        assert_eq!(score, 0);
    }

    #[test]
    fn test_screen_transaction_clear() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, MockAmlOracleContract);
        let oracle_client = MockAmlOracleContractClient::new(&env, &oracle_id);
        let admin = Address::generate(&env);

        oracle_client.initialize(&admin);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        oracle_client.set_risk_score(&admin, &sender, &20);

        let config = AmlConfig {
            admin: admin.clone(),
            oracle_address: oracle_id,
            risk_threshold: 50,
            enabled: true,
        };

        let result = screen_transaction(&env, &config, &sender, &recipient, 1000).unwrap();
        assert_eq!(result.risk_score, 20);
        assert_eq!(result.status, AmlStatus::Clear);
    }

    #[test]
    fn test_screen_transaction_flagged() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, MockAmlOracleContract);
        let oracle_client = MockAmlOracleContractClient::new(&env, &oracle_id);
        let admin = Address::generate(&env);

        oracle_client.initialize(&admin);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        oracle_client.set_risk_score(&admin, &sender, &80);

        let config = AmlConfig {
            admin: admin.clone(),
            oracle_address: oracle_id,
            risk_threshold: 50,
            enabled: true,
        };

        let result = screen_transaction(&env, &config, &sender, &recipient, 5000).unwrap();
        assert_eq!(result.risk_score, 80);
        assert_eq!(result.status, AmlStatus::Flagged);
    }

    #[test]
    fn test_screen_transaction_disabled() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let admin = Address::generate(&env);

        let config = AmlConfig {
            admin: admin.clone(),
            oracle_address: Address::generate(&env),
            risk_threshold: 50,
            enabled: false,
        };

        let result = screen_transaction(&env, &config, &sender, &recipient, 1000).unwrap();
        assert_eq!(result.risk_score, 0);
        assert_eq!(result.status, AmlStatus::Clear);
    }

    #[test]
    fn test_screen_transaction_oracle_unavailable() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let bogus_oracle = Address::generate(&env);
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let admin = Address::generate(&env);

        let config = AmlConfig {
            admin: admin.clone(),
            oracle_address: bogus_oracle,
            risk_threshold: 50,
            enabled: true,
        };

        let result = screen_transaction(&env, &config, &sender, &recipient, 1000);
        assert_eq!(result, Err(AmlError::OracleUnavailable));
    }

    #[test]
    fn test_screen_returns_higher_score() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, MockAmlOracleContract);
        let oracle_client = MockAmlOracleContractClient::new(&env, &oracle_id);
        let admin = Address::generate(&env);

        oracle_client.initialize(&admin);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        oracle_client.set_risk_score(&admin, &sender, &30);
        oracle_client.set_risk_score(&admin, &recipient, &90);

        let config = AmlConfig {
            admin: admin.clone(),
            oracle_address: oracle_id,
            risk_threshold: 50,
            enabled: true,
        };

        let result = screen_transaction(&env, &config, &sender, &recipient, 1000).unwrap();
        assert_eq!(result.risk_score, 90);
        assert_eq!(result.status, AmlStatus::Flagged);
    }

    #[test]
    fn test_screen_at_threshold_boundary() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, MockAmlOracleContract);
        let oracle_client = MockAmlOracleContractClient::new(&env, &oracle_id);
        let admin = Address::generate(&env);

        oracle_client.initialize(&admin);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        oracle_client.set_risk_score(&admin, &sender, &50);

        let config = AmlConfig {
            admin: admin.clone(),
            oracle_address: oracle_id,
            risk_threshold: 50,
            enabled: true,
        };

        let result = screen_transaction(&env, &config, &sender, &recipient, 1000).unwrap();
        assert_eq!(result.risk_score, 50);
        assert_eq!(result.status, AmlStatus::Clear);
    }
}
