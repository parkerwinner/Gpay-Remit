use soroban_sdk::{
    contract, contracterror, contractimpl, contracttype, symbol_short, Address, Env, IntoVal,
    InvokeError, String, Symbol, Val, Vec,
};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum OracleError {
    OracleNotConfigured = 1,
    OracleTimeout = 2,
    InvalidRate = 3,
    AssetNotSupported = 4,
    StaleRate = 5,
    Unauthorized = 6,
    RateLimitExceeded = 7,
    ConversionOverflow = 8,
    InvalidAmount = 9,
    FallbackFailed = 10,
    SameAsset = 11,
}

#[derive(Clone)]
#[contracttype]
pub struct OracleConfig {
    pub primary_oracle: Address,
    pub secondary_oracle: Address,
    pub admin: Address,
    pub max_staleness: u64,
    pub rate_limit_interval: u64,
    pub last_query_ledger: u64,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct ConversionResult {
    pub converted_amount: i128,
    pub rate: i128,
    pub denominator: i128,
    pub from_asset: String,
    pub to_asset: String,
    pub timestamp: u64,
}

#[derive(Clone)]
#[contracttype]
pub struct CachedRate {
    pub rate: i128,
    pub denominator: i128,
    pub timestamp: u64,
    pub from_asset: String,
    pub to_asset: String,
}

#[derive(Clone)]
#[contracttype]
pub enum OracleDataKey {
    Config,
    CachedRate(String, String),
    SupportedPair(String, String),
}

const RATE_PRECISION: i128 = 1_000_000_000_000_000_000; // 18 decimal places

#[contract]
pub struct MockOracleContract;

#[contractimpl]
impl MockOracleContract {
    pub fn initialize(env: Env, admin: Address) {
        admin.require_auth();
        env.storage()
            .instance()
            .set(&Symbol::new(&env, "admin"), &admin);
    }

    pub fn set_rate(
        env: Env,
        admin: Address,
        from_asset: String,
        to_asset: String,
        rate: i128,
        denominator: i128,
    ) {
        admin.require_auth();
        let stored_admin: Address = env
            .storage()
            .instance()
            .get(&Symbol::new(&env, "admin"))
            .unwrap();
        if admin != stored_admin {
            panic!("unauthorized");
        }
        if rate <= 0 || denominator <= 0 {
            panic!("invalid rate");
        }
        let key = OracleDataKey::CachedRate(from_asset, to_asset);
        let cached = CachedRate {
            rate,
            denominator,
            timestamp: env.ledger().timestamp(),
            from_asset: String::from_str(&env, ""),
            to_asset: String::from_str(&env, ""),
        };
        env.storage().instance().set(&key, &cached);
    }

    pub fn query_rate(env: Env, from_asset: String, to_asset: String) -> CachedRate {
        let key = OracleDataKey::CachedRate(from_asset.clone(), to_asset.clone());
        let cached: Option<CachedRate> = env.storage().instance().get(&key);
        match cached {
            Some(mut c) => {
                c.from_asset = from_asset;
                c.to_asset = to_asset;
                c
            }
            None => panic!("rate not available"),
        }
    }
}

pub fn get_conversion_rate(
    env: &Env,
    oracle_address: &Address,
    from_asset: &String,
    to_asset: &String,
    amount: i128,
    max_staleness: u64,
    cached_rate: Option<CachedRate>,
) -> Result<ConversionResult, OracleError> {
    if amount <= 0 {
        return Err(OracleError::InvalidAmount);
    }

    if from_asset == to_asset {
        return Ok(ConversionResult {
            converted_amount: amount,
            rate: RATE_PRECISION,
            denominator: RATE_PRECISION,
            from_asset: from_asset.clone(),
            to_asset: to_asset.clone(),
            timestamp: env.ledger().timestamp(),
        });
    }

    let oracle_result = query_oracle(env, oracle_address, from_asset, to_asset);

    match oracle_result {
        Ok(rate_data) => {
            validate_rate(&rate_data, env.ledger().timestamp(), max_staleness)?;
            let converted = apply_conversion(amount, rate_data.rate, rate_data.denominator)?;

            env.events().publish(
                (symbol_short!("conv"), symbol_short!("rate")),
                (
                    from_asset.clone(),
                    to_asset.clone(),
                    rate_data.rate,
                    converted,
                ),
            );

            Ok(ConversionResult {
                converted_amount: converted,
                rate: rate_data.rate,
                denominator: rate_data.denominator,
                from_asset: from_asset.clone(),
                to_asset: to_asset.clone(),
                timestamp: env.ledger().timestamp(),
            })
        }
        Err(_) => match cached_rate {
            Some(ref cache) => {
                let staleness_limit = max_staleness.checked_mul(3).unwrap_or(max_staleness);
                validate_rate(cache, env.ledger().timestamp(), staleness_limit)?;
                let converted = apply_conversion(amount, cache.rate, cache.denominator)?;

                env.events().publish(
                    (symbol_short!("conv"), symbol_short!("cache")),
                    (from_asset.clone(), to_asset.clone(), cache.rate, converted),
                );

                Ok(ConversionResult {
                    converted_amount: converted,
                    rate: cache.rate,
                    denominator: cache.denominator,
                    from_asset: from_asset.clone(),
                    to_asset: to_asset.clone(),
                    timestamp: cache.timestamp,
                })
            }
            None => Err(OracleError::FallbackFailed),
        },
    }
}

fn query_oracle(
    env: &Env,
    oracle_address: &Address,
    from_asset: &String,
    to_asset: &String,
) -> Result<CachedRate, OracleError> {
    let func = Symbol::new(env, "query_rate");
    let args: Vec<Val> = Vec::from_array(env, [from_asset.into_val(env), to_asset.into_val(env)]);
    match env.try_invoke_contract::<CachedRate, InvokeError>(oracle_address, &func, args) {
        Ok(Ok(rate)) => Ok(rate),
        _ => Err(OracleError::OracleTimeout),
    }
}

fn validate_rate(
    rate_data: &CachedRate,
    current_timestamp: u64,
    max_staleness: u64,
) -> Result<(), OracleError> {
    if rate_data.rate <= 0 {
        return Err(OracleError::InvalidRate);
    }
    if rate_data.denominator <= 0 {
        return Err(OracleError::InvalidRate);
    }

    if max_staleness > 0 && rate_data.timestamp > 0 {
        let age = current_timestamp.saturating_sub(rate_data.timestamp);
        if age > max_staleness {
            return Err(OracleError::StaleRate);
        }
    }

    Ok(())
}

fn apply_conversion(amount: i128, rate: i128, denominator: i128) -> Result<i128, OracleError> {
    if denominator == 0 {
        return Err(OracleError::InvalidRate);
    }
    let result = amount
        .checked_mul(rate)
        .ok_or(OracleError::ConversionOverflow)?
        .checked_div(denominator)
        .ok_or(OracleError::ConversionOverflow)?;

    if result < 0 {
        return Err(OracleError::InvalidRate);
    }

    Ok(result)
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    #[test]
    fn test_mock_oracle_set_and_query() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, MockOracleContract);
        let client = MockOracleContractClient::new(&env, &contract_id);
        let admin = Address::generate(&env);

        client.initialize(&admin);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        client.set_rate(&admin, &from, &to, &920000, &1000000);

        let result = client.query_rate(&from, &to);
        assert_eq!(result.rate, 920000);
        assert_eq!(result.denominator, 1000000);
        assert_eq!(result.timestamp, 1000);
    }

    #[test]
    fn test_same_asset_conversion() {
        let env = Env::default();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_addr = Address::generate(&env);
        let asset = String::from_str(&env, "USDC");

        let result =
            get_conversion_rate(&env, &oracle_addr, &asset, &asset, 5000, 3600, None).unwrap();

        assert_eq!(result.converted_amount, 5000);
    }

    #[test]
    fn test_invalid_amount() {
        let env = Env::default();
        let oracle_addr = Address::generate(&env);
        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        let result = get_conversion_rate(&env, &oracle_addr, &from, &to, 0, 3600, None);
        assert_eq!(result, Err(OracleError::InvalidAmount));

        let result = get_conversion_rate(&env, &oracle_addr, &from, &to, -100, 3600, None);
        assert_eq!(result, Err(OracleError::InvalidAmount));
    }

    #[test]
    fn test_apply_conversion_math() {
        let result = apply_conversion(1000, 920000, 1000000).unwrap();
        assert_eq!(result, 920);

        let result = apply_conversion(10000, 85_000_000, 100_000_000).unwrap();
        assert_eq!(result, 8500);
    }

    #[test]
    fn test_apply_conversion_zero_denominator() {
        let result = apply_conversion(1000, 920000, 0);
        assert_eq!(result, Err(OracleError::InvalidRate));
    }

    #[test]
    fn test_validate_rate_stale() {
        let rate = CachedRate {
            rate: 920000,
            denominator: 1000000,
            timestamp: 100,
            from_asset: soroban_sdk::String::from_str(&soroban_sdk::Env::default(), ""),
            to_asset: soroban_sdk::String::from_str(&soroban_sdk::Env::default(), ""),
        };
        let result = validate_rate(&rate, 5000, 3600);
        assert_eq!(result, Err(OracleError::StaleRate));
    }

    #[test]
    fn test_validate_rate_valid() {
        let rate = CachedRate {
            rate: 920000,
            denominator: 1000000,
            timestamp: 3000,
            from_asset: soroban_sdk::String::from_str(&soroban_sdk::Env::default(), ""),
            to_asset: soroban_sdk::String::from_str(&soroban_sdk::Env::default(), ""),
        };
        let result = validate_rate(&rate, 5000, 3600);
        assert!(result.is_ok());
    }

    #[test]
    fn test_validate_rate_negative() {
        let rate = CachedRate {
            rate: -1,
            denominator: 1000000,
            timestamp: 1000,
            from_asset: soroban_sdk::String::from_str(&soroban_sdk::Env::default(), ""),
            to_asset: soroban_sdk::String::from_str(&soroban_sdk::Env::default(), ""),
        };
        let result = validate_rate(&rate, 1000, 3600);
        assert_eq!(result, Err(OracleError::InvalidRate));
    }

    #[test]
    fn test_oracle_cross_contract_call() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, MockOracleContract);
        let oracle_client = MockOracleContractClient::new(&env, &oracle_id);
        let admin = Address::generate(&env);

        oracle_client.initialize(&admin);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        oracle_client.set_rate(&admin, &from, &to, &920000, &1000000);

        let result = get_conversion_rate(&env, &oracle_id, &from, &to, 1000, 3600, None).unwrap();

        assert_eq!(result.rate, 920000);
        assert_eq!(result.denominator, 1000000);
        assert_eq!(result.converted_amount, 920);
    }

    #[test]
    fn test_fallback_to_cached_rate() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let bogus_oracle = Address::generate(&env);
        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        let cached = CachedRate {
            rate: 910000,
            denominator: 1000000,
            timestamp: 800,
            from_asset: from.clone(),
            to_asset: to.clone(),
        };

        let result = get_conversion_rate(&env, &bogus_oracle, &from, &to, 1000, 3600, Some(cached));

        assert!(result.is_ok());
        let conversion = result.unwrap();
        assert_eq!(conversion.converted_amount, 910);
        assert_eq!(conversion.rate, 910000);
    }
}
