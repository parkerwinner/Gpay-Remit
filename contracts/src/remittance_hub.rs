use soroban_sdk::{contract, contractimpl, contracttype, contracterror, Address, Env, String, Symbol, symbol_short};
use crate::oracle::{self, CachedRate, OracleConfig};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum RemittanceError {
    InvalidAmount = 1,
    NotFound = 2,
    InvalidStatus = 3,
    DueDateInPast = 4,
    MissingEscrow = 5,
    InvoiceNotFound = 6,
    InvalidInvoiceStatus = 7,
    Unauthorized = 8,
    OracleNotConfigured = 9,
    OracleTimeout = 10,
    InvalidRate = 11,
    AssetNotSupported = 12,
    StaleRate = 13,
    ConversionFailed = 14,
    RateLimitExceeded = 15,
    AlreadyInitialized = 16,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum InvoiceStatus {
    Unpaid,
    Paid,
    Overdue,
    Cancelled,
}

#[derive(Clone)]
#[contracttype]
pub struct Asset {
    pub code: String,
    pub issuer: Address,
}

#[derive(Clone)]
#[contracttype]
pub struct Invoice {
    pub invoice_id: u64,
    pub sender: Address,
    pub recipient: Address,
    pub amount: i128,
    pub asset: Asset,
    pub converted_amount: i128,
    pub fees: i128,
    pub total_due: i128,
    pub status: InvoiceStatus,
    pub created_at: u64,
    pub due_date: u64,
    pub paid_at: u64,
    pub description: String,
    pub escrow_id: u64,
    pub memo: String,
}

#[derive(Clone)]
#[contracttype]
pub struct RemittanceData {
    pub from: Address,
    pub to: Address,
    pub amount: i128,
    pub currency: Symbol,
    pub status: Symbol,
}

#[derive(Clone, Copy)]
#[contracttype]
pub enum DataKey {
    InvoiceCounter,
    Invoice(u64),
    EscrowInvoice(u64),
    Admin,
}

#[derive(Clone)]
#[contracttype]
pub enum HubOracleKey {
    OracleConfig,
    CachedRate(String, String),
}

#[contract]
pub struct RemittanceHubContract;

#[contractimpl]
impl RemittanceHubContract {
    pub fn initialize(
        env: Env,
        admin: Address,
        primary_oracle: Address,
        secondary_oracle: Address,
        max_staleness: u64,
    ) -> Result<(), RemittanceError> {
        admin.require_auth();

        if env.storage().persistent().has(&DataKey::Admin) {
            return Err(RemittanceError::AlreadyInitialized);
        }

        env.storage().persistent().set(&DataKey::Admin, &admin);

        let config = OracleConfig {
            primary_oracle,
            secondary_oracle,
            admin: admin.clone(),
            max_staleness,
            rate_limit_interval: 5,
            last_query_ledger: 0,
        };
        env.storage().persistent().set(&HubOracleKey::OracleConfig, &config);

        env.events().publish(
            (symbol_short!("hub_init"),),
            admin,
        );

        Ok(())
    }

    pub fn set_oracle(
        env: Env,
        caller: Address,
        primary_oracle: Address,
        secondary_oracle: Address,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();
        let stored_admin: Address = env.storage().persistent()
            .get(&DataKey::Admin)
            .ok_or(RemittanceError::OracleNotConfigured)?;
        if caller != stored_admin {
            return Err(RemittanceError::Unauthorized);
        }

        let mut config: OracleConfig = env.storage().persistent()
            .get(&HubOracleKey::OracleConfig)
            .ok_or(RemittanceError::OracleNotConfigured)?;

        config.primary_oracle = primary_oracle.clone();
        config.secondary_oracle = secondary_oracle.clone();
        env.storage().persistent().set(&HubOracleKey::OracleConfig, &config);

        env.events().publish(
            (symbol_short!("orc_set"),),
            (primary_oracle, secondary_oracle),
        );

        Ok(())
    }

    pub fn set_max_staleness(
        env: Env,
        caller: Address,
        max_staleness: u64,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();
        let stored_admin: Address = env.storage().persistent()
            .get(&DataKey::Admin)
            .ok_or(RemittanceError::OracleNotConfigured)?;
        if caller != stored_admin {
            return Err(RemittanceError::Unauthorized);
        }

        let mut config: OracleConfig = env.storage().persistent()
            .get(&HubOracleKey::OracleConfig)
            .ok_or(RemittanceError::OracleNotConfigured)?;

        config.max_staleness = max_staleness;
        env.storage().persistent().set(&HubOracleKey::OracleConfig, &config);

        Ok(())
    }

    pub fn set_cached_rate(
        env: Env,
        caller: Address,
        from_asset: String,
        to_asset: String,
        rate: i128,
        denominator: i128,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();
        let stored_admin: Address = env.storage().persistent()
            .get(&DataKey::Admin)
            .ok_or(RemittanceError::OracleNotConfigured)?;
        if caller != stored_admin {
            return Err(RemittanceError::Unauthorized);
        }
        if rate <= 0 || denominator <= 0 {
            return Err(RemittanceError::InvalidRate);
        }

        let cached = CachedRate {
            rate,
            denominator,
            timestamp: env.ledger().timestamp(),
            from_asset: from_asset.clone(),
            to_asset: to_asset.clone(),
        };
        env.storage().persistent().set(
            &HubOracleKey::CachedRate(from_asset, to_asset),
            &cached,
        );

        Ok(())
    }

    pub fn get_oracle_config(env: Env) -> Option<OracleConfig> {
        env.storage().persistent().get(&HubOracleKey::OracleConfig)
    }

    pub fn send_remittance(
        env: Env,
        from: Address,
        to: Address,
        amount: i128,
        currency: Symbol,
    ) -> Result<u64, RemittanceError> {
        from.require_auth();

        if amount <= 0 {
            return Err(RemittanceError::InvalidAmount);
        }

        let remittance_id = env.ledger().sequence() as u64;
        
        let remittance = RemittanceData {
            from: from.clone(),
            to,
            amount,
            currency,
            status: symbol_short!("pending"),
        };

        env.storage()
            .persistent()
            .set(&remittance_id, &remittance);

        Ok(remittance_id)
    }

    pub fn convert_currency(
        env: Env,
        amount: i128,
        from_asset: String,
        to_asset: String,
    ) -> Result<oracle::ConversionResult, RemittanceError> {
        if amount <= 0 {
            return Err(RemittanceError::InvalidAmount);
        }

        let config: OracleConfig = env.storage().persistent()
            .get(&HubOracleKey::OracleConfig)
            .ok_or(RemittanceError::OracleNotConfigured)?;

        let cached: Option<CachedRate> = env.storage().persistent()
            .get(&HubOracleKey::CachedRate(from_asset.clone(), to_asset.clone()));

        let result = oracle::get_conversion_rate(
            &env,
            &config.primary_oracle,
            &from_asset,
            &to_asset,
            amount,
            config.max_staleness,
            cached.clone(),
        );

        match result {
            Ok(conversion) => {
                let new_cache = CachedRate {
                    rate: conversion.rate,
                    denominator: conversion.denominator,
                    timestamp: conversion.timestamp,
                    from_asset: from_asset.clone(),
                    to_asset: to_asset.clone(),
                };
                env.storage().persistent().set(
                    &HubOracleKey::CachedRate(from_asset, to_asset),
                    &new_cache,
                );
                Ok(conversion)
            }
            Err(_) => {
                let secondary_result = oracle::get_conversion_rate(
                    &env,
                    &config.secondary_oracle,
                    &from_asset,
                    &to_asset,
                    amount,
                    config.max_staleness,
                    cached,
                );
                match secondary_result {
                    Ok(conversion) => {
                        let new_cache = CachedRate {
                            rate: conversion.rate,
                            denominator: conversion.denominator,
                            timestamp: conversion.timestamp,
                            from_asset: from_asset.clone(),
                            to_asset: to_asset.clone(),
                        };
                        env.storage().persistent().set(
                            &HubOracleKey::CachedRate(from_asset, to_asset),
                            &new_cache,
                        );
                        Ok(conversion)
                    }
                    Err(_) => Err(RemittanceError::ConversionFailed),
                }
            }
        }
    }

    pub fn complete_remittance(env: Env, remittance_id: u64, caller: Address) -> Result<(), RemittanceError> {
        caller.require_auth();

        let mut remittance: RemittanceData = env
            .storage()
            .persistent()
            .get(&remittance_id)
            .ok_or(RemittanceError::NotFound)?;

        if remittance.status != symbol_short!("pending") {
            return Err(RemittanceError::InvalidStatus);
        }

        remittance.status = symbol_short!("complete");
        env.storage().persistent().set(&remittance_id, &remittance);

        Ok(())
    }

    pub fn get_remittance(env: Env, remittance_id: u64) -> Option<RemittanceData> {
        env.storage().persistent().get(&remittance_id)
    }

    pub fn generate_invoice(
        env: Env,
        sender: Address,
        recipient: Address,
        amount: i128,
        asset: Asset,
        due_date: u64,
        description: String,
        escrow_id: u64,
        memo: String,
    ) -> Result<u64, RemittanceError> {
        sender.require_auth();

        if amount <= 0 {
            return Err(RemittanceError::InvalidAmount);
        }

        let current_time = env.ledger().timestamp();
        if due_date <= current_time {
            return Err(RemittanceError::DueDateInPast);
        }

        let mut counter: u64 = env.storage().persistent().get(&DataKey::InvoiceCounter).unwrap_or(0);
        counter = counter.checked_add(1).unwrap_or(counter);

        let converted_amount = Self::convert_with_oracle(&env, amount, &asset.code);

        let fee_percentage = 250;
        let fees = amount.checked_mul(fee_percentage)
            .unwrap_or(0)
            .checked_div(10000)
            .unwrap_or(0);

        let total_due = amount.checked_add(fees).unwrap_or(amount);

        let invoice = Invoice {
            invoice_id: counter,
            sender: sender.clone(),
            recipient,
            amount,
            asset,
            converted_amount,
            fees,
            total_due,
            status: InvoiceStatus::Unpaid,
            created_at: current_time,
            due_date,
            paid_at: 0,
            description,
            escrow_id,
            memo,
        };

        env.storage().persistent().set(&DataKey::Invoice(counter), &invoice);
        env.storage().persistent().set(&DataKey::InvoiceCounter, &counter);

        if escrow_id > 0 {
            env.storage().persistent().set(&DataKey::EscrowInvoice(escrow_id), &counter);
        }

        env.events().publish(
            (symbol_short!("inv_gen"), counter),
            (sender, amount, total_due, due_date)
        );

        Ok(counter)
    }

    pub fn get_invoice(env: Env, invoice_id: u64) -> Option<Invoice> {
        env.storage().persistent().get(&DataKey::Invoice(invoice_id))
    }

    pub fn get_invoice_by_escrow(env: Env, escrow_id: u64) -> Option<u64> {
        env.storage().persistent().get(&DataKey::EscrowInvoice(escrow_id))
    }

    pub fn mark_invoice_paid(
        env: Env,
        invoice_id: u64,
        caller: Address,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();

        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        if invoice.status == InvoiceStatus::Paid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        if caller != invoice.sender && caller != invoice.recipient {
            return Err(RemittanceError::Unauthorized);
        }

        invoice.status = InvoiceStatus::Paid;
        invoice.paid_at = env.ledger().timestamp();

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_paid"), invoice_id),
            (caller, invoice.paid_at)
        );

        Ok(())
    }

    pub fn mark_invoice_overdue(
        env: Env,
        invoice_id: u64,
    ) -> Result<(), RemittanceError> {
        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        let current_time = env.ledger().timestamp();
        
        if current_time <= invoice.due_date {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        if invoice.status == InvoiceStatus::Paid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        invoice.status = InvoiceStatus::Overdue;

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_over"), invoice_id),
            current_time
        );

        Ok(())
    }

    pub fn cancel_invoice(
        env: Env,
        invoice_id: u64,
        caller: Address,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();

        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        if caller != invoice.sender {
            return Err(RemittanceError::Unauthorized);
        }

        if invoice.status == InvoiceStatus::Paid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        invoice.status = InvoiceStatus::Cancelled;

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_canc"), invoice_id),
            caller
        );

        Ok(())
    }

    pub fn update_invoice_amount(
        env: Env,
        invoice_id: u64,
        caller: Address,
        new_amount: i128,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();

        if new_amount <= 0 {
            return Err(RemittanceError::InvalidAmount);
        }

        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        if caller != invoice.sender {
            return Err(RemittanceError::Unauthorized);
        }

        if invoice.status != InvoiceStatus::Unpaid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        let fee_percentage = 250;
        let fees = new_amount.checked_mul(fee_percentage)
            .unwrap_or(0)
            .checked_div(10000)
            .unwrap_or(0);

        invoice.amount = new_amount;
        invoice.fees = fees;
        invoice.total_due = new_amount.checked_add(fees).unwrap_or(new_amount);

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_upd"), invoice_id),
            (caller, new_amount, invoice.total_due)
        );

        Ok(())
    }

    pub fn get_conversion_rate(
        env: Env,
        from_asset: String,
        to_asset: String,
        amount: i128,
    ) -> Result<oracle::ConversionResult, RemittanceError> {
        Self::convert_currency(env, amount, from_asset, to_asset)
    }

    fn convert_with_oracle(env: &Env, amount: i128, asset_code: &String) -> i128 {
        let target = String::from_str(env, "USD");
        if asset_code == &target {
            return amount;
        }

        let config: Option<OracleConfig> = env.storage().persistent()
            .get(&HubOracleKey::OracleConfig);

        match config {
            Some(cfg) => {
                let cached: Option<CachedRate> = env.storage().persistent()
                    .get(&HubOracleKey::CachedRate(asset_code.clone(), target.clone()));

                let result = oracle::get_conversion_rate(
                    env,
                    &cfg.primary_oracle,
                    asset_code,
                    &target,
                    amount,
                    cfg.max_staleness,
                    cached,
                );
                match result {
                    Ok(conversion) => conversion.converted_amount,
                    Err(_) => amount,
                }
            }
            None => amount,
        }
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

    #[test]
    fn test_send_remittance() {
        let env = Env::default();
        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let from = Address::generate(&env);
        let to = Address::generate(&env);

        env.mock_all_auths();
        let remittance_id = client.send_remittance(&from, &to, &5000, &symbol_short!("USD"));

        let remittance = client.get_remittance(&remittance_id);
        assert!(remittance.is_some());
    }

    #[test]
    fn test_generate_invoice() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment for services"),
            &0,
            &String::from_str(&env, "Remittance memo")
        );

        assert_eq!(invoice_id, 1);

        let invoice = client.get_invoice(&invoice_id);
        assert!(invoice.is_some());

        let invoice_data = invoice.unwrap();
        assert_eq!(invoice_data.amount, 1000);
        assert_eq!(invoice_data.status, InvoiceStatus::Unpaid);
        assert_eq!(invoice_data.sender, sender);
        assert_eq!(invoice_data.recipient, recipient);
    }

    #[test]
    fn test_mark_invoice_paid() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        env.ledger().with_mut(|li| {
            li.timestamp = 1500;
        });

        client.mark_invoice_paid(&invoice_id, &sender);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.status, InvoiceStatus::Paid);
        assert_eq!(invoice.paid_at, 1500);
    }

    #[test]
    fn test_mark_invoice_overdue() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        client.mark_invoice_overdue(&invoice_id);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.status, InvoiceStatus::Overdue);
    }

    #[test]
    fn test_cancel_invoice() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        client.cancel_invoice(&invoice_id, &sender);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.status, InvoiceStatus::Cancelled);
    }

    #[test]
    fn test_update_invoice_amount() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        client.update_invoice_amount(&invoice_id, &sender, &1500);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.amount, 1500);
        let expected_fee = 1500 * 250 / 10000;
        assert_eq!(invoice.fees, expected_fee);
        assert_eq!(invoice.total_due, 1500 + expected_fee);
    }

    #[test]
    fn test_invoice_with_escrow_link() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let escrow_id = 123;
        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &escrow_id,
            &String::from_str(&env, "Memo")
        );

        let linked_invoice_id = client.get_invoice_by_escrow(&escrow_id);
        assert!(linked_invoice_id.is_some());
        assert_eq!(linked_invoice_id.unwrap(), invoice_id);
    }

    #[test]
    fn test_invoice_due_date_validation() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 2000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let result = client.try_generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &1500,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        assert_eq!(result, Err(Ok(RemittanceError::DueDateInPast)));
    }

    #[test]
    fn test_initialize_hub() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let primary_oracle = Address::generate(&env);
        let secondary_oracle = Address::generate(&env);

        client.initialize(&admin, &primary_oracle, &secondary_oracle, &3600);

        let config = client.get_oracle_config();
        assert!(config.is_some());
        let cfg = config.unwrap();
        assert_eq!(cfg.admin, admin);
        assert_eq!(cfg.primary_oracle, primary_oracle);
        assert_eq!(cfg.secondary_oracle, secondary_oracle);
        assert_eq!(cfg.max_staleness, 3600);
    }

    #[test]
    fn test_initialize_double_init() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let oracle = Address::generate(&env);

        client.initialize(&admin, &oracle, &oracle, &3600);

        let result = client.try_initialize(&admin, &oracle, &oracle, &3600);
        assert_eq!(result, Err(Ok(RemittanceError::AlreadyInitialized)));
    }

    #[test]
    fn test_set_oracle_addresses() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let primary = Address::generate(&env);
        let secondary = Address::generate(&env);
        let new_primary = Address::generate(&env);
        let new_secondary = Address::generate(&env);

        client.initialize(&admin, &primary, &secondary, &3600);
        client.set_oracle(&admin, &new_primary, &new_secondary);

        let config = client.get_oracle_config().unwrap();
        assert_eq!(config.primary_oracle, new_primary);
        assert_eq!(config.secondary_oracle, new_secondary);
    }

    #[test]
    fn test_set_oracle_unauthorized() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let oracle = Address::generate(&env);
        let other = Address::generate(&env);

        client.initialize(&admin, &oracle, &oracle, &3600);

        let result = client.try_set_oracle(&other, &oracle, &oracle);
        assert_eq!(result, Err(Ok(RemittanceError::Unauthorized)));
    }

    #[test]
    fn test_set_cached_rate() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let oracle = Address::generate(&env);

        client.initialize(&admin, &oracle, &oracle, &3600);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        client.set_cached_rate(&admin, &from, &to, &920000, &1000000);
    }

    #[test]
    fn test_set_cached_rate_invalid() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let oracle = Address::generate(&env);

        client.initialize(&admin, &oracle, &oracle, &3600);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        let result = client.try_set_cached_rate(&admin, &from, &to, &0, &1000000);
        assert_eq!(result, Err(Ok(RemittanceError::InvalidRate)));

        let result = client.try_set_cached_rate(&admin, &from, &to, &920000, &-1);
        assert_eq!(result, Err(Ok(RemittanceError::InvalidRate)));
    }

    #[test]
    fn test_convert_currency_with_oracle() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, crate::oracle::MockOracleContract);
        let oracle_client = crate::oracle::MockOracleContractClient::new(&env, &oracle_id);
        let oracle_admin = Address::generate(&env);
        oracle_client.initialize(&oracle_admin);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");
        oracle_client.set_rate(&oracle_admin, &from, &to, &920000, &1000000);

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin, &oracle_id, &oracle_id, &3600);

        let result = client.convert_currency(&1000, &from, &to);
        assert_eq!(result.converted_amount, 920);
        assert_eq!(result.rate, 920000);
        assert_eq!(result.denominator, 1000000);
    }

    #[test]
    fn test_convert_currency_same_asset() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, crate::oracle::MockOracleContract);
        let oracle_client = crate::oracle::MockOracleContractClient::new(&env, &oracle_id);
        let oracle_admin = Address::generate(&env);
        oracle_client.initialize(&oracle_admin);

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin, &oracle_id, &oracle_id, &3600);

        let asset = String::from_str(&env, "USDC");
        let result = client.convert_currency(&5000, &asset, &asset);
        assert_eq!(result.converted_amount, 5000);
    }

    #[test]
    fn test_convert_currency_invalid_amount() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let oracle = Address::generate(&env);
        client.initialize(&admin, &oracle, &oracle, &3600);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        let result = client.try_convert_currency(&0, &from, &to);
        assert_eq!(result, Err(Ok(RemittanceError::InvalidAmount)));
    }

    #[test]
    fn test_convert_currency_no_oracle_config() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");

        let result = client.try_convert_currency(&1000, &from, &to);
        assert_eq!(result, Err(Ok(RemittanceError::OracleNotConfigured)));
    }

    #[test]
    fn test_convert_currency_fallback_to_secondary() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let bogus_primary = Address::generate(&env);

        let secondary_id = env.register_contract(None, crate::oracle::MockOracleContract);
        let secondary_client = crate::oracle::MockOracleContractClient::new(&env, &secondary_id);
        let oracle_admin = Address::generate(&env);
        secondary_client.initialize(&oracle_admin);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");
        secondary_client.set_rate(&oracle_admin, &from, &to, &910000, &1000000);

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin, &bogus_primary, &secondary_id, &3600);

        let cached = CachedRate {
            rate: 900000,
            denominator: 1000000,
            timestamp: 800,
            from_asset: from.clone(),
            to_asset: to.clone(),
        };
        client.set_cached_rate(&admin, &from, &to, &cached.rate, &cached.denominator);

        let result = client.convert_currency(&1000, &from, &to);
        assert_eq!(result.converted_amount, 900);
    }

    #[test]
    fn test_set_max_staleness() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        let oracle = Address::generate(&env);

        client.initialize(&admin, &oracle, &oracle, &3600);
        client.set_max_staleness(&admin, &7200);

        let config = client.get_oracle_config().unwrap();
        assert_eq!(config.max_staleness, 7200);
    }

    #[test]
    fn test_get_conversion_rate() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, crate::oracle::MockOracleContract);
        let oracle_client = crate::oracle::MockOracleContractClient::new(&env, &oracle_id);
        let oracle_admin = Address::generate(&env);
        oracle_client.initialize(&oracle_admin);

        let from = String::from_str(&env, "USDC");
        let to = String::from_str(&env, "EUR");
        oracle_client.set_rate(&oracle_admin, &from, &to, &850000, &1000000);

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin, &oracle_id, &oracle_id, &3600);

        let result = client.get_conversion_rate(&from, &to, &10000);
        assert_eq!(result.converted_amount, 8500);
        assert_eq!(result.rate, 850000);
    }

    #[test]
    fn test_generate_invoice_with_oracle_conversion() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let oracle_id = env.register_contract(None, crate::oracle::MockOracleContract);
        let oracle_client = crate::oracle::MockOracleContractClient::new(&env, &oracle_id);
        let oracle_admin = Address::generate(&env);
        oracle_client.initialize(&oracle_admin);

        let from = String::from_str(&env, "EUR");
        let to = String::from_str(&env, "USD");
        oracle_client.set_rate(&oracle_admin, &from, &to, &1_080_000, &1_000_000);

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let admin = Address::generate(&env);
        client.initialize(&admin, &oracle_id, &oracle_id, &3600);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "EUR"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Cross-border payment"),
            &0,
            &String::from_str(&env, "Memo"),
        );

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.amount, 1000);
        assert_eq!(invoice.converted_amount, 1080);
    }
}
