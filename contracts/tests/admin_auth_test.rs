//! Authorization tests for every admin-only function of the remittance hub (#365).
//!
//! For each admin function we verify that:
//! - an authenticated caller that is NOT the stored admin is rejected with
//!   `Unauthorized`,
//! - the real admin cannot call it without signing (`require_auth` is enforced),
//! - the call fails closed when the hub has not been initialized.

use gpay_remit_contracts::rate_limit::{FunctionType, RateLimitConfig};
use gpay_remit_contracts::remittance_hub::{
    FeeConfig, MetricType, RemittanceError, RemittanceHubContract, RemittanceHubContractClient,
};
use gpay_remit_contracts::upgradeable::UpgradeError;
use soroban_sdk::{testutils::Address as _, Address, BytesN, Env, String};

struct Setup<'a> {
    env: Env,
    client: RemittanceHubContractClient<'a>,
    admin: Address,
    attacker: Address,
}

fn setup<'a>() -> Setup<'a> {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, RemittanceHubContract);
    let client = RemittanceHubContractClient::new(&env, &contract_id);
    let admin = Address::generate(&env);
    let oracle = Address::generate(&env);
    client.init_hub(&admin, &oracle, &oracle, &3600);
    let attacker = Address::generate(&env);
    Setup {
        env,
        client,
        admin,
        attacker,
    }
}

fn uninitialized<'a>() -> (Env, RemittanceHubContractClient<'a>, Address) {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, RemittanceHubContract);
    let client = RemittanceHubContractClient::new(&env, &contract_id);
    let caller = Address::generate(&env);
    (env, client, caller)
}

fn fee_config() -> FeeConfig {
    FeeConfig {
        percentage: 100,
        min_fee: 0,
        max_fee: 1_000,
    }
}

fn rl_config() -> RateLimitConfig {
    RateLimitConfig {
        enabled: true,
        max_count: 1,
        interval: 60,
    }
}

const UNAUTH: Result<RemittanceError, soroban_sdk::InvokeError> = Ok(RemittanceError::Unauthorized);

// ── Non-admin callers are rejected ───────────────────────────────────

#[test]
fn non_admin_cannot_set_oracle() {
    let s = setup();
    let r = s
        .client
        .try_set_oracle(&s.attacker, &s.attacker, &s.attacker);
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert_ne!(
        s.client.get_oracle_config().unwrap().primary_oracle,
        s.attacker
    );
}

#[test]
fn non_admin_cannot_set_max_staleness() {
    let s = setup();
    let r = s.client.try_set_max_staleness(&s.attacker, &1);
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert_eq!(s.client.get_oracle_config().unwrap().max_staleness, 3600);
}

#[test]
fn non_admin_cannot_set_cached_rate() {
    let s = setup();
    let from = String::from_str(&s.env, "USDC");
    let to = String::from_str(&s.env, "EUR");
    let r = s
        .client
        .try_set_cached_rate(&s.attacker, &from, &to, &1, &1);
    assert_eq!(r.unwrap_err(), UNAUTH);
}

#[test]
fn non_admin_cannot_set_fee_config() {
    let s = setup();
    let r = s.client.try_set_fee_config(&s.attacker, &fee_config());
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert!(s.client.get_fee_config().is_none());
}

#[test]
fn non_admin_cannot_configure_aml() {
    let s = setup();
    let r = s.client.try_configure_aml(&s.attacker, &s.attacker, &0);
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert!(s.client.get_aml_config().is_none());
}

#[test]
fn non_admin_cannot_set_aml_threshold() {
    let s = setup();
    let oracle = Address::generate(&s.env);
    s.client.configure_aml(&s.admin, &oracle, &70);
    let r = s.client.try_set_aml_threshold(&s.attacker, &100);
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert_eq!(s.client.get_aml_config().unwrap().risk_threshold, 70);
}

#[test]
fn non_admin_cannot_set_aml_oracle() {
    let s = setup();
    let oracle = Address::generate(&s.env);
    s.client.configure_aml(&s.admin, &oracle, &70);
    let r = s.client.try_set_aml_oracle(&s.attacker, &s.attacker);
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert_eq!(s.client.get_aml_config().unwrap().oracle_address, oracle);
}

#[test]
fn non_admin_cannot_clear_aml_flag() {
    let s = setup();
    let r = s.client.try_clear_aml_flag(&s.attacker, &1);
    assert_eq!(r.unwrap_err(), UNAUTH);
}

#[test]
fn non_admin_cannot_set_max_batch_size() {
    let s = setup();
    let r = s.client.try_set_max_batch_size(&s.attacker, &1_000);
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert_eq!(s.client.get_max_batch_size(), 10);
}

#[test]
fn non_admin_cannot_set_rate_limit_config() {
    let s = setup();
    let r =
        s.client
            .try_set_rate_limit_config(&s.attacker, &FunctionType::Remittance, &rl_config());
    assert_eq!(r.unwrap_err(), UNAUTH);
}

#[test]
fn non_admin_cannot_set_global_rate_limit_config() {
    let s = setup();
    let r = s.client.try_set_global_rate_limit_config(
        &s.attacker,
        &FunctionType::Remittance,
        &rl_config(),
    );
    assert_eq!(r.unwrap_err(), UNAUTH);
}

#[test]
fn non_admin_cannot_set_rate_limit_exemption() {
    let s = setup();
    let r = s
        .client
        .try_set_rate_limit_exemption(&s.attacker, &s.attacker, &true);
    assert_eq!(r.unwrap_err(), UNAUTH);
    assert!(!s.client.is_rate_limit_exempt(&s.attacker));
}

#[test]
fn non_admin_cannot_reset_metric() {
    let s = setup();
    let r = s
        .client
        .try_reset_metric(&s.attacker, &MetricType::Volume, &0, &false);
    assert_eq!(r.unwrap_err(), UNAUTH);
}

#[test]
fn non_admin_cannot_pause() {
    let s = setup();
    let r = s.client.try_pause(&s.attacker);
    assert_eq!(r.unwrap_err(), Ok(UpgradeError::Unauthorized));
    assert!(!s.client.is_paused());
}

#[test]
fn non_admin_cannot_unpause() {
    let s = setup();
    let r = s.client.try_unpause(&s.attacker);
    assert_eq!(r.unwrap_err(), Ok(UpgradeError::Unauthorized));
}

#[test]
fn non_admin_cannot_upgrade() {
    let s = setup();
    let hash = BytesN::from_array(&s.env, &[0u8; 32]);
    let r = s.client.try_upgrade(&s.attacker, &hash);
    assert_eq!(r.unwrap_err(), Ok(UpgradeError::Unauthorized));
}

#[test]
fn non_admin_cannot_migrate() {
    let s = setup();
    let r = s.client.try_migrate(&s.attacker);
    assert_eq!(r.unwrap_err(), Ok(UpgradeError::Unauthorized));
}

// ── The admin must actually sign (require_auth is enforced) ──────────

/// Initializes the hub with mocked auths, then removes all mocks so that the
/// following call carries no signatures.
fn setup_without_auth<'a>() -> Setup<'a> {
    let s = setup();
    s.env.set_auths(&[]);
    s
}

#[test]
fn admin_without_signature_cannot_set_oracle() {
    let s = setup_without_auth();
    assert!(s
        .client
        .try_set_oracle(&s.admin, &s.admin, &s.admin)
        .is_err());
}

#[test]
fn admin_without_signature_cannot_set_max_staleness() {
    let s = setup_without_auth();
    assert!(s.client.try_set_max_staleness(&s.admin, &1).is_err());
}

#[test]
fn admin_without_signature_cannot_set_cached_rate() {
    let s = setup_without_auth();
    let from = String::from_str(&s.env, "USDC");
    let to = String::from_str(&s.env, "EUR");
    assert!(s
        .client
        .try_set_cached_rate(&s.admin, &from, &to, &1, &1)
        .is_err());
}

#[test]
fn admin_without_signature_cannot_set_fee_config() {
    let s = setup_without_auth();
    assert!(s
        .client
        .try_set_fee_config(&s.admin, &fee_config())
        .is_err());
}

#[test]
fn admin_without_signature_cannot_configure_aml() {
    let s = setup_without_auth();
    assert!(s.client.try_configure_aml(&s.admin, &s.admin, &0).is_err());
}

#[test]
fn admin_without_signature_cannot_set_aml_threshold() {
    let s = setup_without_auth();
    assert!(s.client.try_set_aml_threshold(&s.admin, &1).is_err());
}

#[test]
fn admin_without_signature_cannot_set_aml_oracle() {
    let s = setup_without_auth();
    assert!(s.client.try_set_aml_oracle(&s.admin, &s.admin).is_err());
}

#[test]
fn admin_without_signature_cannot_clear_aml_flag() {
    let s = setup_without_auth();
    assert!(s.client.try_clear_aml_flag(&s.admin, &1).is_err());
}

#[test]
fn admin_without_signature_cannot_set_max_batch_size() {
    let s = setup_without_auth();
    assert!(s.client.try_set_max_batch_size(&s.admin, &1).is_err());
}

#[test]
fn admin_without_signature_cannot_set_rate_limit_config() {
    let s = setup_without_auth();
    assert!(s
        .client
        .try_set_rate_limit_config(&s.admin, &FunctionType::Remittance, &rl_config())
        .is_err());
}

#[test]
fn admin_without_signature_cannot_set_global_rate_limit_config() {
    let s = setup_without_auth();
    assert!(s
        .client
        .try_set_global_rate_limit_config(&s.admin, &FunctionType::Remittance, &rl_config())
        .is_err());
}

#[test]
fn admin_without_signature_cannot_set_rate_limit_exemption() {
    let s = setup_without_auth();
    assert!(s
        .client
        .try_set_rate_limit_exemption(&s.admin, &s.admin, &true)
        .is_err());
}

#[test]
fn admin_without_signature_cannot_reset_metric() {
    let s = setup_without_auth();
    assert!(s
        .client
        .try_reset_metric(&s.admin, &MetricType::Volume, &0, &false)
        .is_err());
}

#[test]
fn admin_without_signature_cannot_pause() {
    let s = setup_without_auth();
    assert!(s.client.try_pause(&s.admin).is_err());
}

// ── Uninitialized hub fails closed (no panic, no takeover) ───────────

#[test]
fn uninitialized_hub_rejects_admin_functions() {
    let (env, client, caller) = uninitialized();
    let from = String::from_str(&env, "USDC");
    let to = String::from_str(&env, "EUR");

    assert_eq!(
        client
            .try_set_oracle(&caller, &caller, &caller)
            .unwrap_err(),
        Ok(RemittanceError::OracleNotConfigured)
    );
    assert_eq!(
        client.try_set_max_staleness(&caller, &1).unwrap_err(),
        Ok(RemittanceError::OracleNotConfigured)
    );
    assert_eq!(
        client
            .try_set_cached_rate(&caller, &from, &to, &1, &1)
            .unwrap_err(),
        Ok(RemittanceError::OracleNotConfigured)
    );
    assert_eq!(
        client
            .try_set_fee_config(&caller, &fee_config())
            .unwrap_err(),
        UNAUTH
    );
    assert_eq!(
        client.try_configure_aml(&caller, &caller, &0).unwrap_err(),
        UNAUTH
    );
    assert_eq!(
        client.try_set_aml_threshold(&caller, &1).unwrap_err(),
        UNAUTH
    );
    assert_eq!(
        client.try_set_aml_oracle(&caller, &caller).unwrap_err(),
        UNAUTH
    );
    assert_eq!(client.try_clear_aml_flag(&caller, &1).unwrap_err(), UNAUTH);
    assert_eq!(
        client.try_set_max_batch_size(&caller, &1).unwrap_err(),
        UNAUTH
    );
    assert_eq!(
        client
            .try_set_rate_limit_config(&caller, &FunctionType::Remittance, &rl_config())
            .unwrap_err(),
        UNAUTH
    );
    assert_eq!(
        client
            .try_set_global_rate_limit_config(&caller, &FunctionType::Remittance, &rl_config())
            .unwrap_err(),
        UNAUTH
    );
    assert_eq!(
        client
            .try_set_rate_limit_exemption(&caller, &caller, &true)
            .unwrap_err(),
        UNAUTH
    );
    assert_eq!(
        client
            .try_reset_metric(&caller, &MetricType::Volume, &0, &false)
            .unwrap_err(),
        UNAUTH
    );
}

#[test]
fn uninitialized_hub_rejects_upgrade_functions_without_panicking() {
    let (env, client, caller) = uninitialized();
    let hash = BytesN::from_array(&env, &[0u8; 32]);
    let unauth = Ok(UpgradeError::Unauthorized);
    assert_eq!(client.try_pause(&caller).unwrap_err(), unauth);
    assert_eq!(client.try_unpause(&caller).unwrap_err(), unauth);
    assert_eq!(client.try_upgrade(&caller, &hash).unwrap_err(), unauth);
    assert_eq!(client.try_migrate(&caller).unwrap_err(), unauth);
}

// ── The real admin is still allowed ──────────────────────────────────

#[test]
fn admin_can_call_admin_functions() {
    let s = setup();
    let oracle = Address::generate(&s.env);
    s.client.set_oracle(&s.admin, &oracle, &oracle);
    s.client.set_max_staleness(&s.admin, &60);
    s.client.set_fee_config(&s.admin, &fee_config());
    s.client.configure_aml(&s.admin, &oracle, &50);
    s.client.set_aml_threshold(&s.admin, &60);
    s.client.set_aml_oracle(&s.admin, &oracle);
    s.client.set_max_batch_size(&s.admin, &20);
    s.client
        .set_rate_limit_config(&s.admin, &FunctionType::Remittance, &rl_config());
    s.client
        .set_global_rate_limit_config(&s.admin, &FunctionType::Remittance, &rl_config());
    s.client
        .set_rate_limit_exemption(&s.admin, &s.attacker, &true);
    s.client
        .reset_metric(&s.admin, &MetricType::Volume, &0, &false);

    assert_eq!(s.client.get_oracle_config().unwrap().max_staleness, 60);
    assert_eq!(s.client.get_aml_config().unwrap().risk_threshold, 60);
    assert_eq!(s.client.get_max_batch_size(), 20);
    assert!(s.client.is_rate_limit_exempt(&s.attacker));
}
