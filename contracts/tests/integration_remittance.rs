/// Integration tests for the full remittance flow on a simulated Testnet environment.
///
/// These tests exercise the end-to-end lifecycle:
///   create escrow → deposit → verify conditions → release
///   create escrow → deposit → refund / dispute
///
/// Run with: cargo test --features integration
#[cfg(feature = "integration")]
mod integration_remittance {
    use gpay_remit_contracts::payment_escrow::{
        Asset, EscrowStatus, Error, PaymentEscrowContract, PaymentEscrowContractClient,
    };
    use soroban_sdk::{
        testutils::{Address as _, Ledger},
        token, Address, Env, String,
    };

    // ── Helpers ─────────────────────────────────────────────────────────────

    fn create_token<'a>(
        env: &Env,
        admin: &Address,
    ) -> (token::Client<'a>, token::StellarAssetClient<'a>) {
        let addr = env.register_stellar_asset_contract_v2(admin.clone());
        (
            token::Client::new(env, &addr.address()),
            token::StellarAssetClient::new(env, &addr.address()),
        )
    }

    fn setup<'a>(
        env: &Env,
    ) -> (
        PaymentEscrowContractClient<'a>,
        Address,
        Address,
        Address,
        token::Client<'a>,
        token::StellarAssetClient<'a>,
        Asset,
    ) {
        env.mock_all_auths();
        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(env, &contract_id);

        let admin = Address::generate(env);
        let sender = Address::generate(env);
        let recipient = Address::generate(env);

        client.init_escrow(&admin);

        let (token, token_admin) = create_token(env, &admin);
        let asset = Asset {
            code: String::from_str(env, "USDC"),
            issuer: admin.clone(),
        };
        client.add_supported_asset(&admin, &asset);

        (client, admin, sender, recipient, token, token_admin, asset)
    }

    // ── Full happy-path flow ─────────────────────────────────────────────────

    /// Create escrow → full deposit → release by recipient.
    #[test]
    fn test_full_flow_create_deposit_release() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let amount: i128 = 5_000;
        token_admin.mint(&sender, &amount);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &10_000,
            &String::from_str(&env, "integration-test-memo"),
        );

        // Deposit full amount
        client.deposit(&escrow_id, &sender, &amount, &token.address);

        let escrow = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(escrow.status, EscrowStatus::Funded);
        assert_eq!(escrow.deposited_amount, amount);
        assert_eq!(token.balance(&client.address), amount);

        // Release
        let pre_balance = token.balance(&recipient);
        client.release_escrow(&escrow_id, &recipient, &token.address);

        let post = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(post.status, EscrowStatus::Released);
        assert_eq!(token.balance(&recipient), pre_balance + amount);
    }

    /// Create escrow → deposit → refund by sender after expiry.
    #[test]
    fn test_full_flow_create_deposit_refund_on_expiry() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        let now: u64 = 1_000;
        env.ledger().with_mut(|li| li.timestamp = now);

        let amount: i128 = 2_000;
        token_admin.mint(&sender, &amount);

        let expiration = now + 500;
        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &expiration,
            &String::from_str(&env, "refund-test"),
        );

        client.deposit(&escrow_id, &sender, &amount, &token.address);

        // Advance ledger past expiry
        env.ledger().with_mut(|li| li.timestamp = expiration + 1);

        let pre_balance = token.balance(&sender);
        client.refund_escrow(&escrow_id, &sender, &token.address, &gpay_remit_contracts::payment_escrow::RefundReason::Expiration);

        let post = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(post.status, EscrowStatus::Refunded);
        assert_eq!(token.balance(&sender), pre_balance + amount);
    }

    /// Partial deposits accumulate until full; then release.
    #[test]
    fn test_full_flow_partial_deposits_then_release() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let amount: i128 = 3_000;
        token_admin.mint(&sender, &amount);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &9_000,
            &String::from_str(&env, "partial-deposit"),
        );

        // First partial deposit
        client.deposit(&escrow_id, &sender, &1_000, &token.address);
        let mid = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(mid.status, EscrowStatus::Pending);
        assert_eq!(mid.deposited_amount, 1_000);

        // Second partial — completes funding
        client.deposit(&escrow_id, &sender, &2_000, &token.address);
        let funded = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(funded.status, EscrowStatus::Funded);

        client.release_escrow(&escrow_id, &recipient, &token.address);
        let released = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(released.status, EscrowStatus::Released);
    }

    // ── Error / edge-case scenarios ──────────────────────────────────────────

    /// Deposit by wrong sender is rejected.
    #[test]
    fn test_flow_deposit_wrong_sender_rejected() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        env.ledger().with_mut(|li| li.timestamp = 1_000);
        let amount: i128 = 1_000;
        let attacker = Address::generate(&env);
        token_admin.mint(&attacker, &amount);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &5_000,
            &String::from_str(&env, ""),
        );

        let result = client.try_deposit(&escrow_id, &attacker, &amount, &token.address);
        assert_eq!(result, Err(Ok(Error::WrongSender)));
    }

    /// Deposit that exceeds the required amount is rejected.
    #[test]
    fn test_flow_excess_deposit_rejected() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        env.ledger().with_mut(|li| li.timestamp = 1_000);
        let amount: i128 = 1_000;
        token_admin.mint(&sender, &5_000);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &5_000,
            &String::from_str(&env, ""),
        );

        let result = client.try_deposit(&escrow_id, &sender, &(amount + 1), &token.address);
        assert!(result.is_err());
    }

    /// Release of an unfunded escrow is rejected.
    #[test]
    fn test_flow_release_unfunded_rejected() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, _token_admin, asset) = setup(&env);

        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &1_000,
            &asset,
            &5_000,
            &String::from_str(&env, ""),
        );

        let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
        assert!(result.is_err());
    }

    /// Refund before expiry is rejected.
    #[test]
    fn test_flow_refund_before_expiry_rejected() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        let now: u64 = 1_000;
        env.ledger().with_mut(|li| li.timestamp = now);
        let amount: i128 = 1_000;
        token_admin.mint(&sender, &amount);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &(now + 10_000),
            &String::from_str(&env, ""),
        );

        client.deposit(&escrow_id, &sender, &amount, &token.address);

        let result = client.try_refund_escrow(&escrow_id, &sender, &token.address, &gpay_remit_contracts::payment_escrow::RefundReason::Expiration);
        assert_eq!(result, Err(Ok(Error::NotExpired)));
    }

    // ── High-value scenario ──────────────────────────────────────────────────

    #[test]
    fn test_flow_high_value_transfer() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let amount: i128 = 100_000_000; // 100M units
        token_admin.mint(&sender, &amount);

        let escrow_id = client.create_escrow(
            &sender,
            &recipient,
            &amount,
            &asset,
            &50_000,
            &String::from_str(&env, "high-value"),
        );

        client.deposit(&escrow_id, &sender, &amount, &token.address);
        client.release_escrow(&escrow_id, &recipient, &token.address);

        let post = client.get_escrow(&escrow_id).unwrap();
        assert_eq!(post.status, EscrowStatus::Released);
        assert_eq!(token.balance(&recipient), amount);
    }

    // ── Multi-escrow isolation ───────────────────────────────────────────────

    /// Two concurrent escrows are independent — releasing one does not affect the other.
    #[test]
    fn test_flow_two_concurrent_escrows_isolated() {
        let env = Env::default();
        let (client, _admin, sender, recipient, token, token_admin, asset) = setup(&env);

        env.ledger().with_mut(|li| li.timestamp = 1_000);

        let a1: i128 = 1_000;
        let a2: i128 = 2_000;
        token_admin.mint(&sender, &(a1 + a2));

        let id1 = client.create_escrow(
            &sender,
            &recipient,
            &a1,
            &asset,
            &9_000,
            &String::from_str(&env, "escrow-a"),
        );
        let id2 = client.create_escrow(
            &sender,
            &recipient,
            &a2,
            &asset,
            &9_000,
            &String::from_str(&env, "escrow-b"),
        );

        client.deposit(&id1, &sender, &a1, &token.address);
        client.deposit(&id2, &sender, &a2, &token.address);

        client.release_escrow(&id1, &recipient, &token.address);

        let e1 = client.get_escrow(&id1).unwrap();
        let e2 = client.get_escrow(&id2).unwrap();
        assert_eq!(e1.status, EscrowStatus::Released);
        assert_eq!(e2.status, EscrowStatus::Funded);
    }
}
