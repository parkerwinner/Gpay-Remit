//! Comprehensive contract upgrade tests
//! Covers: data migration, backward compatibility, versioning, pause, auth, and paused-state enforcement.
//! Uses RemittanceHubContract which wires `upgradeable` helpers.

use gpay_remit_contracts::remittance_hub::{
    Asset, InvoiceStatus, MetricType, RemittanceHubContract, RemittanceHubContractClient,
};
use gpay_remit_contracts::upgradeable::{self, UpgradeError};
use soroban_sdk::{
    symbol_short,
    testutils::{Address as _, Ledger},
    Address, BytesN, Env, String, Symbol,
};

fn setup_hub<'a>(env: &Env) -> (RemittanceHubContractClient<'a>, Address, Address, Address) {
    env.mock_all_auths();
    let contract_id = env.register_contract(None, RemittanceHubContract);
    let client = RemittanceHubContractClient::new(env, &contract_id);
    let admin = Address::generate(env);
    let user1 = Address::generate(env);
    let user2 = Address::generate(env);
    client.init_hub(&admin, &admin, &admin, &300);
    (client, admin, user1, user2)
}

fn dummy_wasm_hash(env: &Env) -> BytesN<32> {
    BytesN::from_array(env, &[42u8; 32])
}

fn dummy_wasm_hash2(env: &Env) -> BytesN<32> {
    BytesN::from_array(env, &[99u8; 32])
}

// ---------------------------------------------------------------------------
// Versioning
// ---------------------------------------------------------------------------

#[test]
fn test_initial_version_is_one() {
    let env = Env::default();
    let (client, _admin, _u1, _u2) = setup_hub(&env);
    assert_eq!(client.version(), 1);
}

#[test]
fn test_upgrade_increments_version_and_pauses() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    assert_eq!(client.version(), 1);
    assert!(!client.is_paused());

    let hash = dummy_wasm_hash(&env);
    client.upgrade(&admin, &hash);

    assert_eq!(client.version(), 2);
    assert!(client.is_paused());
}

#[test]
fn test_consecutive_upgrades_increment_version() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    client.upgrade(&admin, &dummy_wasm_hash(&env));
    assert_eq!(client.version(), 2);
    // migrate to unpause before next upgrade
    client.migrate(&admin);
    assert!(!client.is_paused());
    assert_eq!(client.version(), 2);

    client.upgrade(&admin, &dummy_wasm_hash2(&env));
    assert_eq!(client.version(), 3);
    assert!(client.is_paused());
}

#[test]
fn test_migrate_unpauses_and_returns_version() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    client.upgrade(&admin, &dummy_wasm_hash(&env));
    assert!(client.is_paused());
    let v = client.migrate(&admin);
    assert_eq!(v, 2);
    assert!(!client.is_paused());
    // version unchanged by migrate
    assert_eq!(client.version(), 2);
}

#[test]
fn test_upgrade_unauthorized_fails() {
    let env = Env::default();
    let (client, _admin, user1, _u2) = setup_hub(&env);
    let hash = dummy_wasm_hash(&env);
    let res = client.try_upgrade(&user1, &hash);
    assert_eq!(res, Err(Ok(UpgradeError::Unauthorized)));
    // version must not have changed
    assert_eq!(client.version(), 1);
    assert!(!client.is_paused());
}

#[test]
fn test_migrate_unauthorized_fails() {
    let env = Env::default();
    let (client, admin, user1, _u2) = setup_hub(&env);
    client.upgrade(&admin, &dummy_wasm_hash(&env));
    let res = client.try_migrate(&user1);
    assert_eq!(res, Err(Ok(UpgradeError::Unauthorized)));
    // still paused
    assert!(client.is_paused());
}

#[test]
fn test_pause_unpause_flow() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    assert!(!client.is_paused());
    client.pause(&admin);
    assert!(client.is_paused());
    client.unpause(&admin);
    assert!(!client.is_paused());
}

#[test]
fn test_pause_already_paused_fails() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    client.pause(&admin);
    let res = client.try_pause(&admin);
    assert_eq!(res, Err(Ok(UpgradeError::AlreadyPaused)));
}

#[test]
fn test_unpause_not_paused_fails() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    let res = client.try_unpause(&admin);
    assert_eq!(res, Err(Ok(UpgradeError::NotPaused)));
}

#[test]
fn test_pause_unauthorized_fails() {
    let env = Env::default();
    let (client, _admin, user1, _u2) = setup_hub(&env);
    let res = client.try_pause(&user1);
    assert_eq!(res, Err(Ok(UpgradeError::Unauthorized)));
}

#[test]
fn test_unpause_unauthorized_fails() {
    let env = Env::default();
    let (client, admin, user1, _u2) = setup_hub(&env);
    client.pause(&admin);
    let res = client.try_unpause(&user1);
    assert_eq!(res, Err(Ok(UpgradeError::Unauthorized)));
}

// ---------------------------------------------------------------------------
// Paused-state enforcement (backward compatibility with existing behavior)
// ---------------------------------------------------------------------------

#[test]
fn test_send_remittance_blocked_when_paused() {
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    client.pause(&admin);
    let res = client.try_send_remittance(&user1, &user2, &1000, &symbol_short!("USD"));
    // RemittanceHub returns RemittanceError::ContractPaused = 32
    assert!(res.is_err());
}

#[test]
fn test_generate_invoice_blocked_when_paused() {
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    env.ledger().with_mut(|li| li.timestamp = 1000);
    client.pause(&admin);
    let issuer = Address::generate(&env);
    let asset = Asset {
        code: String::from_str(&env, "USDC"),
        issuer,
    };
    let res = client.try_generate_invoice(
        &user1,
        &user2,
        &1000,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
        &0,
        &String::from_str(&env, "memo"),
    );
    assert!(res.is_err());
}

#[test]
fn test_operations_resume_after_migrate() {
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    env.ledger().with_mut(|li| li.timestamp = 1000);
    client.upgrade(&admin, &dummy_wasm_hash(&env));
    assert!(client.is_paused());
    client.migrate(&admin);
    assert!(!client.is_paused());

    // Should succeed now
    env.ledger().with_mut(|li| li.sequence_number = 1);
    let id = client.send_remittance(&user1, &user2, &1000, &symbol_short!("USD"));
    assert!(client.get_remittance(&id).is_some());

    let issuer = Address::generate(&env);
    let asset = Asset {
        code: String::from_str(&env, "USDC"),
        issuer,
    };
    let invoice_id = client.generate_invoice(
        &user1,
        &user2,
        &500,
        &asset,
        &2000,
        &String::from_str(&env, "after-migrate"),
        &0,
        &String::from_str(&env, ""),
    );
    assert_eq!(invoice_id, 1);
}

// ---------------------------------------------------------------------------
// Data migration & persistence (CRITICAL)
// ---------------------------------------------------------------------------

#[test]
fn test_invoice_data_persists_across_upgrade() {
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    env.ledger().with_mut(|li| li.timestamp = 1000);
    let issuer = Address::generate(&env);
    let asset = Asset {
        code: String::from_str(&env, "USDC"),
        issuer: issuer.clone(),
    };
    let due = 2000;
    let inv1 = client.generate_invoice(
        &user1,
        &user2,
        &1000,
        &asset,
        &due,
        &String::from_str(&env, "Invoice 1"),
        &0,
        &String::from_str(&env, "memo1"),
    );
    let inv2 = client.generate_invoice(
        &user1,
        &user2,
        &2000,
        &asset,
        &due,
        &String::from_str(&env, "Invoice 2"),
        &11,
        &String::from_str(&env, "memo2"),
    );
    assert_eq!(inv1, 1);
    assert_eq!(inv2, 2);

    // Upgrade
    client.upgrade(&admin, &dummy_wasm_hash(&env));
    client.migrate(&admin);

    // Data must still be readable - backward compatibility
    let fetched1 = client.get_invoice(&inv1).expect("invoice 1 must persist");
    let fetched2 = client.get_invoice(&inv2).expect("invoice 2 must persist");
    assert_eq!(fetched1.amount, 1000);
    assert_eq!(fetched2.amount, 2000);
    assert_eq!(fetched1.status, InvoiceStatus::Unpaid);
    assert_eq!(fetched2.escrow_id, 11);
    // escrow index mapping must also persist
    assert_eq!(client.get_invoice_by_escrow(&11), Some(inv2));
}

#[test]
fn test_remittance_data_persists_across_upgrade() {
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    env.ledger().with_mut(|li| {
        li.timestamp = 1000;
        li.sequence_number = 10;
    });
    let id = client.send_remittance(&user1, &user2, &7500, &symbol_short!("EUR"));
    let before = client
        .get_remittance(&id)
        .expect("must exist before upgrade");

    client.upgrade(&admin, &dummy_wasm_hash(&env));
    client.migrate(&admin);

    let after = client
        .get_remittance(&id)
        .expect("must exist after upgrade");
    assert_eq!(before.amount, after.amount);
    assert_eq!(before.from, after.from);
    assert_eq!(before.to, after.to);
}

#[test]
fn test_metrics_persist_across_upgrade() {
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    let ts = 86400 * 5;
    env.ledger().with_mut(|li| li.timestamp = ts);
    let issuer = Address::generate(&env);
    let asset = Asset {
        code: String::from_str(&env, "USDC"),
        issuer,
    };
    client.generate_invoice(
        &user1,
        &user2,
        &1000,
        &asset,
        &(ts + 1000),
        &String::from_str(&env, "Metric test"),
        &0,
        &String::from_str(&env, ""),
    );
    let vol_before = client.get_metric(&MetricType::Volume, &ts, &false);
    assert_eq!(vol_before, 1000);

    client.upgrade(&admin, &dummy_wasm_hash(&env));
    client.migrate(&admin);

    let vol_after = client.get_metric(&MetricType::Volume, &ts, &false);
    assert_eq!(vol_after, 1000, "daily volume must survive upgrade");

    // New operations after migrate must increment on top of old values
    env.ledger().with_mut(|li| li.timestamp = ts + 100);
    let issuer2 = Address::generate(&env);
    let asset2 = Asset {
        code: String::from_str(&env, "USDC"),
        issuer: issuer2,
    };
    client.generate_invoice(
        &user1,
        &user2,
        &500,
        &asset2,
        &(ts + 2000),
        &String::from_str(&env, "post-upgrade"),
        &0,
        &String::from_str(&env, ""),
    );
    let vol_final = client.get_metric(&MetricType::Volume, &ts, &false);
    assert_eq!(vol_final, 1500);
}

#[test]
fn test_oracle_config_persists_across_upgrade() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    let new_primary = Address::generate(&env);
    let new_secondary = Address::generate(&env);
    client.set_oracle(&admin, &new_primary, &new_secondary);
    client.set_max_staleness(&admin, &9999);

    client.upgrade(&admin, &dummy_wasm_hash(&env));
    client.migrate(&admin);

    let cfg = client
        .get_oracle_config()
        .expect("oracle config must persist");
    assert_eq!(cfg.primary_oracle, new_primary);
    assert_eq!(cfg.secondary_oracle, new_secondary);
    assert_eq!(cfg.max_staleness, 9999u64);
}

#[test]
fn test_cached_rate_persists_across_upgrade() {
    let env = Env::default();
    let (client, admin, _u1, _u2) = setup_hub(&env);
    client.set_cached_rate(
        &admin,
        &String::from_str(&env, "USD"),
        &String::from_str(&env, "EUR"),
        &85,
        &100,
    );

    client.upgrade(&admin, &dummy_wasm_hash(&env));
    client.migrate(&admin);

    // conversion should still use cached rate after upgrade
    let result = client.convert_currency(
        &1000,
        &String::from_str(&env, "USD"),
        &String::from_str(&env, "EUR"),
    );
    // 1000 * 85 / 100 = 850
    assert_eq!(result.converted_amount, 850);
}

#[test]
fn test_invoice_lifecycle_persists_and_mutable_after_upgrade() {
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    env.ledger().with_mut(|li| li.timestamp = 1000);
    let issuer = Address::generate(&env);
    let asset = Asset {
        code: String::from_str(&env, "USDC"),
        issuer,
    };
    let inv = client.generate_invoice(
        &user1,
        &user2,
        &1000,
        &asset,
        &2000,
        &String::from_str(&env, "lifecycle"),
        &0,
        &String::from_str(&env, ""),
    );

    client.upgrade(&admin, &dummy_wasm_hash(&env));
    client.migrate(&admin);

    // Should be able to mutate the old invoice after upgrade - backward compatibility of Invoice struct
    env.ledger().with_mut(|li| li.timestamp = 1500);
    client.mark_invoice_paid(&inv, &user1);
    let fetched = client.get_invoice(&inv).unwrap();
    assert_eq!(fetched.status, InvoiceStatus::Paid);
    assert_eq!(fetched.paid_at, 1500);
}

#[test]
fn test_escrow_batch_data_persists_across_upgrade() {
    use gpay_remit_contracts::remittance_hub::EscrowRequest;
    let env = Env::default();
    let (client, admin, user1, user2) = setup_hub(&env);
    env.ledger().with_mut(|li| li.timestamp = 1000);
    let issuer = Address::generate(&env);
    let asset = Asset {
        code: String::from_str(&env, "USDC"),
        issuer,
    };
    let mut reqs = soroban_sdk::Vec::new(&env);
    reqs.push_back(EscrowRequest {
        recipient: user2.clone(),
        amount: 1000,
        asset: asset.clone(),
        expiration_timestamp: 5000,
    });
    let ids = client.batch_create_escrows(&user1, &reqs);
    assert_eq!(ids.len(), 1);

    client.upgrade(&admin, &dummy_wasm_hash(&env));
    client.migrate(&admin);

    // After upgrade, escrow counter must not reset, and previous data implies counter was 1
    // Creating another escrow should give id 2 (persistence of EscrowCounter)
    let mut reqs2 = soroban_sdk::Vec::new(&env);
    reqs2.push_back(EscrowRequest {
        recipient: user2.clone(),
        amount: 500,
        asset,
        expiration_timestamp: 6000,
    });
    let ids2 = client.batch_create_escrows(&user1, &reqs2);
    assert_eq!(ids2.get(0).unwrap(), 2);
}

// ---------------------------------------------------------------------------
// Direct upgradeable module unit tests (isolated storage)
// ---------------------------------------------------------------------------

#[test]
fn test_upgradeable_init_version_sets_defaults() {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, RemittanceHubContract);
    // Call upgradeable helpers via contract instance storage
    env.as_contract(&contract_id, || {
        upgradeable::init_version(&env);
        assert_eq!(upgradeable::get_version(&env), 1);
        assert!(!upgradeable::is_paused(&env));
    });
}

#[test]
fn test_upgradeable_pause_and_unpause_events() {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, RemittanceHubContract);
    let admin = Address::generate(&env);
    env.as_contract(&contract_id, || {
        upgradeable::init_version(&env);
    });
    env.as_contract(&contract_id, || {
        assert!(upgradeable::pause(&env, &admin).is_ok());
        assert!(upgradeable::is_paused(&env));
    });
    env.as_contract(&contract_id, || {
        assert_eq!(
            upgradeable::pause(&env, &admin),
            Err(UpgradeError::AlreadyPaused)
        );
    });
    env.as_contract(&contract_id, || {
        assert!(upgradeable::unpause(&env, &admin).is_ok());
        assert!(!upgradeable::is_paused(&env));
    });
    env.as_contract(&contract_id, || {
        assert_eq!(
            upgradeable::unpause(&env, &admin),
            Err(UpgradeError::NotPaused)
        );
    });
}

#[test]
fn test_upgradeable_upgrade_and_migrate_cycle() {
    let env = Env::default();
    env.mock_all_auths();
    let contract_id = env.register_contract(None, RemittanceHubContract);
    let admin = Address::generate(&env);
    let hash = BytesN::from_array(&env, &[1u8; 32]);
    env.as_contract(&contract_id, || {
        upgradeable::init_version(&env);
        assert_eq!(upgradeable::get_version(&env), 1);
    });
    env.as_contract(&contract_id, || {
        // upgrade should bump version and pause
        assert!(upgradeable::upgrade(&env, &admin, hash.clone()).is_ok());
        assert_eq!(upgradeable::get_version(&env), 2);
        assert!(upgradeable::is_paused(&env));
    });
    env.as_contract(&contract_id, || {
        // migrate should unpause and return version
        let v = upgradeable::migrate(&env, &admin).unwrap();
        assert_eq!(v, 2);
        assert!(!upgradeable::is_paused(&env));
    });
}
