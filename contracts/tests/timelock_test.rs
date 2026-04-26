use gpay_remit_contracts::payment_escrow::{
    Asset, Error, EscrowStatus, PaymentEscrowContract, PaymentEscrowContractClient, RefundReason,
};
use soroban_sdk::{
    testutils::{Address as _, Ledger},
    token, Address, Env, String,
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

fn setup_test<'a>(
    env: &Env,
) -> (
    PaymentEscrowContractClient<'a>,
    Address,
    Address,
    Address,
    (token::Client<'a>, token::StellarAssetClient<'a>),
    Asset,
) {
    env.mock_all_auths();
    let contract_id = env.register_contract(None, PaymentEscrowContract);
    let client = PaymentEscrowContractClient::new(env, &contract_id);

    let admin = Address::generate(env);
    let sender = Address::generate(env);
    let recipient = Address::generate(env);

    client.init_escrow(&admin);

    let token = create_token_contract(env, &admin);
    let asset = Asset {
        code: String::from_str(env, "USDC"),
        issuer: admin.clone(),
    };

    client.add_supported_asset(&admin, &asset);

    (client, admin, sender, recipient, token, asset)
}

// Test expiration at exact timestamp boundary
#[test]
fn test_expiration_at_exact_boundary() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time to exact expiration boundary
    env.ledger().with_mut(|li| li.timestamp = expiration);

    // At exact expiration (using > comparison), release should still succeed (not expired yet)
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert!(result.is_ok());

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Released);
}

// Test expiration one second before
#[test]
fn test_expiration_one_second_before() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time to one second before expiration
    env.ledger().with_mut(|li| li.timestamp = expiration - 1);

    // Release should succeed (not expired yet)
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert!(result.is_ok());

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Released);
}

// Test expiration one second after
#[test]
fn test_expiration_one_second_after() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time to one second after expiration
    env.ledger().with_mut(|li| li.timestamp = expiration + 1);

    // Release should fail (expired)
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert_eq!(result, Err(Ok(Error::Expired)));

    // Refund should succeed after expiration
    let refund_result = client.try_refund_escrow(
        &escrow_id,
        &sender,
        &token.address,
        &RefundReason::Expiration,
    );
    assert!(refund_result.is_ok());
}

// Test release before expiration (should succeed)
#[test]
fn test_release_before_expiration() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time well before expiration
    env.ledger().with_mut(|li| li.timestamp = 1500);

    // Release should succeed
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert!(result.is_ok());

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Released);
    assert!(escrow.released_amount > 0);
}

// Test refund after expiration (should succeed)
#[test]
fn test_refund_after_expiration() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time after expiration
    env.ledger().with_mut(|li| li.timestamp = expiration + 100);

    // Refund should succeed
    let result = client.try_refund_escrow(
        &escrow_id,
        &sender,
        &token.address,
        &RefundReason::Expiration,
    );
    assert!(result.is_ok());

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Refunded);
    assert!(escrow.refunded_amount > 0);
}

// Test refund before expiration with non-expiration reason (should succeed)
#[test]
fn test_refund_before_expiration_sender_request() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time before expiration
    env.ledger().with_mut(|li| li.timestamp = 1500);

    // Refund with SenderRequest reason should succeed even before expiration
    let result = client.try_refund_escrow(
        &escrow_id,
        &sender,
        &token.address,
        &RefundReason::SenderRequest,
    );
    assert!(result.is_ok());

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Refunded);
}

// Test refund before expiration with expiration reason (should fail)
#[test]
fn test_refund_before_expiration_with_expiration_reason() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time before expiration
    env.ledger().with_mut(|li| li.timestamp = 1500);

    // Refund with Expiration reason should fail before expiration
    let result = client.try_refund_escrow(
        &escrow_id,
        &sender,
        &token.address,
        &RefundReason::Expiration,
    );
    assert_eq!(result, Err(Ok(Error::NotExpired)));
}

// Test timestamp overflow scenarios
#[test]
fn test_timestamp_overflow() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let max_timestamp = u64::MAX;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    // Create escrow with maximum timestamp
    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &max_timestamp,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Release should succeed (not expired)
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert!(result.is_ok());
}

// Test zero expiration (no time lock)
#[test]
fn test_zero_expiration() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 0u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // With zero expiration, release should fail (already expired)
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert_eq!(result, Err(Ok(Error::Expired)));

    // Refund should succeed
    let refund_result = client.try_refund_escrow(
        &escrow_id,
        &sender,
        &token.address,
        &RefundReason::Expiration,
    );
    assert!(refund_result.is_ok());
}

// Test far future expiration
#[test]
fn test_far_future_expiration() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let far_future = u64::MAX - 1000; // Very far in the future

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &far_future,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Release should succeed (not expired)
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert!(result.is_ok());

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Released);
}

// Test multiple escrows with different expiration times
#[test]
fn test_multiple_escrows_different_expirations() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &(amount * 3));

    // Create three escrows with different expirations
    let escrow_id1 = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &1500,
        &String::from_str(&env, "Test1"),
    );
    let escrow_id2 = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test2"),
    );
    let escrow_id3 = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2500,
        &String::from_str(&env, "Test3"),
    );

    client.deposit(&escrow_id1, &sender, &amount, &token.address);
    client.deposit(&escrow_id2, &sender, &amount, &token.address);
    client.deposit(&escrow_id3, &sender, &amount, &token.address);

    // Set time to 1750 (after first, before second and third)
    env.ledger().with_mut(|li| li.timestamp = 1750);

    // First should be expired
    let result1 = client.try_release_escrow(&escrow_id1, &recipient, &token.address);
    assert_eq!(result1, Err(Ok(Error::Expired)));

    // Second and third should succeed
    let result2 = client.try_release_escrow(&escrow_id2, &recipient, &token.address);
    assert!(result2.is_ok());

    let result3 = client.try_release_escrow(&escrow_id3, &recipient, &token.address);
    assert!(result3.is_ok());
}

// Test partial release with expiration
#[test]
fn test_partial_release_with_expiration() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);
    client.enable_partial_release(&escrow_id, &sender);

    // Partial release before expiration should succeed
    env.ledger().with_mut(|li| li.timestamp = 1500);
    let result = client.try_release_partial(&escrow_id, &recipient, &token.address, &500);
    assert!(result.is_ok());

    // Partial release after expiration should fail
    env.ledger().with_mut(|li| li.timestamp = 2100);
    let result2 = client.try_release_partial(&escrow_id, &recipient, &token.address, &400);
    assert_eq!(result2, Err(Ok(Error::Expired)));
}

// Test expiration with timestamp at u64::MAX - 1
#[test]
fn test_expiration_near_max_u64() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = u64::MAX - 1;

    env.ledger().with_mut(|li| li.timestamp = u64::MAX - 100);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // At MAX - 2, should not be expired
    env.ledger().with_mut(|li| li.timestamp = u64::MAX - 2);
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    assert!(result.is_ok());
}

// Test expiration boundary with admin refund
#[test]
fn test_admin_refund_after_expiration() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let expiration = 2000u64;

    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &expiration,
        &String::from_str(&env, "Test"),
    );

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Set time after expiration
    env.ledger().with_mut(|li| li.timestamp = expiration + 100);

    // Admin can refund after expiration
    let result = client.try_refund_escrow(
        &escrow_id,
        &admin,
        &token.address,
        &RefundReason::AdminAction,
    );
    assert!(result.is_ok());

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Refunded);
}
