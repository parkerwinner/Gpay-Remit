use gpay_remit_contracts::payment_escrow::{
    Asset, DisputeReason, DisputeStatus, Error, EscrowStatus, PaymentEscrowContract,
    PaymentEscrowContractClient, ResolutionOutcome,
};
use soroban_sdk::{
    testutils::{Address as _, Ledger},
    token, Address, BytesN, Env, String,
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

// Test dispute can be raised by sender
#[test]
fn test_raise_dispute_by_sender() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[1u8; 32]);
    let result = client.try_raise_dispute(
        &escrow_id,
        &sender,
        &DisputeReason::NonDelivery,
        &evidence_hash,
    );

    assert!(result.is_ok());

    let dispute = client.get_dispute(&escrow_id);
    assert!(dispute.is_some());
    let dispute = dispute.unwrap();
    assert_eq!(dispute.disputer, sender);
    assert_eq!(dispute.reason, DisputeReason::NonDelivery);
    assert_eq!(dispute.status, DisputeStatus::Open);
}

// Test dispute can be raised by recipient
#[test]
fn test_raise_dispute_by_recipient() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[2u8; 32]);
    let result = client.try_raise_dispute(
        &escrow_id,
        &recipient,
        &DisputeReason::AmountMismatch,
        &evidence_hash,
    );

    assert!(result.is_ok());

    let dispute = client.get_dispute(&escrow_id);
    assert!(dispute.is_some());
    let dispute = dispute.unwrap();
    assert_eq!(dispute.disputer, recipient);
}

// Test unauthorized party cannot raise dispute
#[test]
fn test_raise_dispute_unauthorized() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let third_party = Address::generate(&env);
    let evidence_hash = BytesN::from_array(&env, &[3u8; 32]);
    let result = client.try_raise_dispute(
        &escrow_id,
        &third_party,
        &DisputeReason::Fraud,
        &evidence_hash,
    );

    assert_eq!(result, Err(Ok(Error::UnauthorizedCaller)));
}

// Test arbitrator can vote on dispute
#[test]
fn test_arbitrator_vote() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[4u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::NonDelivery, &evidence_hash);

    // Admin is automatically added as arbitrator
    let result = client.try_vote_on_dispute(
        &escrow_id,
        &admin,
        &ResolutionOutcome::FavorRecipient,
    );

    assert!(result.is_ok());
}

// Test non-arbitrator cannot vote
#[test]
fn test_non_arbitrator_cannot_vote() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[5u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::NonDelivery, &evidence_hash);

    let non_arbitrator = Address::generate(&env);
    let result = client.try_vote_on_dispute(
        &escrow_id,
        &non_arbitrator,
        &ResolutionOutcome::FavorSender,
    );

    assert_eq!(result, Err(Ok(Error::NotArbitrator)));
}

// Test dispute resolved when quorum reached (favor sender)
#[test]
fn test_dispute_resolved_favor_sender() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[6u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::NonDelivery, &evidence_hash);

    // Admin votes in favor of sender (majority of 1)
    client.vote_on_dispute(&escrow_id, &admin, &ResolutionOutcome::FavorSender);

    let dispute = client.get_dispute(&escrow_id).unwrap();
    assert_eq!(dispute.status, DisputeStatus::Resolved);

    // Escrow should be in Funded state (can be refunded)
    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Funded);
}

// Test dispute resolved when quorum reached (favor recipient)
#[test]
fn test_dispute_resolved_favor_recipient() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[7u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::NonDelivery, &evidence_hash);

    // Admin votes in favor of recipient
    client.vote_on_dispute(&escrow_id, &admin, &ResolutionOutcome::FavorRecipient);

    let dispute = client.get_dispute(&escrow_id).unwrap();
    assert_eq!(dispute.status, DisputeStatus::Resolved);

    // Escrow should be in Approved state (can be released)
    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Approved);
}

// Test admin can resolve dispute directly
#[test]
fn test_admin_resolve_dispute() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[8u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::Fraud, &evidence_hash);

    // Admin resolves directly
    let result = client.try_resolve_dispute(
        &escrow_id,
        &admin,
        &ResolutionOutcome::FavorRecipient,
    );

    assert!(result.is_ok());

    let dispute = client.get_dispute(&escrow_id).unwrap();
    assert_eq!(dispute.status, DisputeStatus::Resolved);
}

// Test non-admin cannot resolve dispute
#[test]
fn test_non_admin_cannot_resolve() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[9u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::Fraud, &evidence_hash);

    let result = client.try_resolve_dispute(
        &escrow_id,
        &sender,
        &ResolutionOutcome::FavorSender,
    );

    assert_eq!(result, Err(Ok(Error::Unauthorized)));
}

// Test cannot raise duplicate dispute
#[test]
fn test_cannot_raise_duplicate_dispute() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[10u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::NonDelivery, &evidence_hash);

    // Try to raise another dispute
    let result = client.try_raise_dispute(
        &escrow_id,
        &sender,
        &DisputeReason::Fraud,
        &evidence_hash,
    );

    assert_eq!(result, Err(Ok(Error::AlreadyDisputed)));
}

// Test arbitrator cannot vote twice
#[test]
fn test_arbitrator_cannot_vote_twice() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[11u8; 32]);
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::NonDelivery, &evidence_hash);

    // First vote
    client.vote_on_dispute(&escrow_id, &admin, &ResolutionOutcome::FavorSender);

    // Try to vote again - should fail
    // Note: After first vote with single arbitrator, dispute is resolved
    // so this will fail with InvalidStatus rather than AlreadyVoted
    let result = client.try_vote_on_dispute(
        &escrow_id,
        &admin,
        &ResolutionOutcome::FavorRecipient,
    );

    assert!(result.is_err());
}

// Test dispute with different reasons
#[test]
fn test_dispute_reasons() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &(amount * 4));

    let mut reasons = soroban_sdk::Vec::new(&env);
    reasons.push_back(DisputeReason::AmountMismatch);
    reasons.push_back(DisputeReason::NonDelivery);
    reasons.push_back(DisputeReason::Fraud);
    reasons.push_back(DisputeReason::Other);

    for (i, reason) in reasons.iter().enumerate() {
        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &2000,
            &String::from_str(&env, &format!("Test {}", i)),
        );
        client.deposit(&escrow_id, &sender, &amount, &token.address);

        let evidence_hash = BytesN::from_array(&env, &[(i as u8 + 12); 32]);
        client.raise_dispute(&escrow_id, &sender, &reason, &evidence_hash);

        let dispute = client.get_dispute(&escrow_id).unwrap();
        assert_eq!(dispute.reason, reason);
    }
}

// Test dispute status transitions
#[test]
fn test_dispute_status_transitions() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test"),
    );
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let evidence_hash = BytesN::from_array(&env, &[13u8; 32]);
    
    // Initially no dispute
    assert!(client.get_dispute(&escrow_id).is_none());

    // Raise dispute - status should be Open
    client.raise_dispute(&escrow_id, &sender, &DisputeReason::NonDelivery, &evidence_hash);
    let dispute = client.get_dispute(&escrow_id).unwrap();
    assert_eq!(dispute.status, DisputeStatus::Open);

    // Vote - status should change to InReview or Resolved
    client.vote_on_dispute(&escrow_id, &admin, &ResolutionOutcome::FavorSender);
    let dispute = client.get_dispute(&escrow_id).unwrap();
    assert_eq!(dispute.status, DisputeStatus::Resolved);
}

// Test escrow status after dispute resolution
#[test]
fn test_escrow_status_after_resolution() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    env.ledger().with_mut(|li| li.timestamp = 1000);
    token_admin.mint(&sender, &(amount * 2));

    // Test favor sender
    let escrow_id1 = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test1"),
    );
    client.deposit(&escrow_id1, &sender, &amount, &token.address);
    let evidence_hash = BytesN::from_array(&env, &[14u8; 32]);
    client.raise_dispute(&escrow_id1, &sender, &DisputeReason::NonDelivery, &evidence_hash);
    client.resolve_dispute(&escrow_id1, &admin, &ResolutionOutcome::FavorSender);
    
    let escrow1 = client.get_escrow(&escrow_id1).unwrap();
    assert_eq!(escrow1.status, EscrowStatus::Funded);

    // Test favor recipient
    let escrow_id2 = client.create_escrow(
        &sender,
        &recipient,
        &amount,
        &asset,
        &2000,
        &String::from_str(&env, "Test2"),
    );
    client.deposit(&escrow_id2, &sender, &amount, &token.address);
    let evidence_hash2 = BytesN::from_array(&env, &[15u8; 32]);
    client.raise_dispute(&escrow_id2, &sender, &DisputeReason::NonDelivery, &evidence_hash2);
    client.resolve_dispute(&escrow_id2, &admin, &ResolutionOutcome::FavorRecipient);
    
    let escrow2 = client.get_escrow(&escrow_id2).unwrap();
    assert_eq!(escrow2.status, EscrowStatus::Approved);
}
