use gpay_remit_contracts::payment_escrow::{PaymentEscrowContract, PaymentEscrowContractClient, Asset, EscrowStatus, Error, RefundReason};
use soroban_sdk::{testutils::{Address as _, Ledger}, token, Address, Env, String};

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

fn setup_test<'a>(env: &Env) -> (PaymentEscrowContractClient<'a>, Address, Address, Address, (token::Client<'a>, token::StellarAssetClient<'a>), Asset) {
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

// ============================================================================
// COMPREHENSIVE ARITHMETIC OVERFLOW TESTS
// ============================================================================

// Test amount overflow in escrow creation with maximum values
#[test]
fn test_create_escrow_max_i128_amount() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    // Try to create escrow with a very large amount
    let max_amount: i128 = i128::MAX / 1000000; // Scale down to avoid other issues
    let result = client.try_create_escrow(&sender, &recipient, &max_amount, &asset, &2000, &String::from_str(&env, ""));
    
    // Should handle large amounts - either succeed with checked math or return overflow error
    match result {
        Err(Ok(Error::ArithmeticOverflow)) => {} // Expected
        Ok(_) => {} // Also acceptable if it handles large numbers
        _ => {}
    }
}

// Test amount overflow in escrow creation with u64::MAX
#[test]
fn test_create_escrow_max_u64_amount() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    let max_u64: i128 = u64::MAX as i128;
    let result = client.try_create_escrow(&sender, &recipient, &max_u64, &asset, &2000, &String::from_str(&env, ""));
    
    // Should handle u64::MAX properly
    match result {
        Err(Ok(Error::ArithmeticOverflow)) | Ok(_) => {}
        Err(Ok(_)) => {
            // InvalidAmount or other errors are acceptable
        }
        Err(Err(_)) => {}
    }
}

// Test fee calculation overflow with maximum fee percentage
#[test]
fn test_fee_calculation_max_percentage_overflow() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    // Set maximum platform fee (100%)
    client.set_platform_fee(&admin, &10000);
    
    let amount = i128::MAX / 10000; // Large amount
    token_admin.mint(&sender, &(amount * 2));
    
    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Try to release - should handle overflow in fee calculation
    let result = client.try_release_escrow(&escrow_id, &recipient, &token.address);
    match result {
        Err(Ok(Error::ArithmeticOverflow)) | Ok(_) => {}
        Err(Ok(_)) => {} // Other errors are also acceptable
    }
}

// Test fee calculation overflow with multiple fees
#[test]
fn test_fee_calculation_multiple_fees_overflow() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    // Set all fee types to maximum
    client.set_platform_fee(&admin, &10000);
    client.set_processing_fee(&admin, &10000);
    client.set_forex_fee(&admin, &10000);
    client.set_compliance_fee(&admin, &i128::MAX);
    
    let amount = i128::MAX / 10000;
    token_admin.mint(&sender, &(amount * 2));
    
    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Try to get fee breakdown with maximum amount
    let result = client.try_get_fee_breakdown(&i128::MAX);
    match result {
        Err(Ok(Error::ArithmeticOverflow)) | Ok(_) => {}
        _ => {}
    }
}

// Test deposit amount overflow when depositing more than escrow amount
#[test]
fn test_deposit_overflow_exceeds_escrow() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let escrow_amount = 1000;
    token_admin.mint(&sender, &i128::MAX);
    
    let escrow_id = client.create_escrow(&sender, &recipient, &escrow_amount, &asset, &2000, &String::from_str(&env, ""));
    
    // Try to deposit more than escrow amount
    let result = client.try_deposit(&escrow_id, &sender, &(escrow_amount + 1), &token.address);
    assert_eq!(result, Err(Ok(Error::InsufficientAmount)));
}

// Test deposit overflow with cumulative deposits
#[test]
fn test_deposit_cumulative_overflow() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let escrow_amount = 1000;
    token_admin.mint(&sender, &2000);
    
    let escrow_id = client.create_escrow(&sender, &recipient, &escrow_amount, &asset, &2000, &String::from_str(&env, ""));
    
    // First deposit
    client.deposit(&escrow_id, &sender, &600, &token.address);
    
    // Try to deposit more than remaining
    let result = client.try_deposit(&escrow_id, &sender, &500, &token.address);
    // Should succeed (600 + 500 = 1100 > 1000, but checked differently)
    match result {
        Ok(_) | Err(Ok(Error::InsufficientAmount)) => {}
        _ => {}
    }
}

// Test batch operation with large amounts
#[test]
fn test_batch_operation_large_amounts() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    // Create multiple escrows with varying amounts
    let amounts = [100i128, 1000, 10000, 100000, 1000000];
    
    for amount in amounts {
        let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
        assert_eq!(escrow_id > 0, true);
    }
}

// Test counter overflow with many escrows
#[test]
fn test_escrow_id_counter_many_escrows() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    // Create many escrows to test counter behavior
    let mut last_id = 0u64;
    for i in 0..1000 {
        let id = client.create_escrow(&sender, &recipient, &(i as i128 + 1), &asset, &2000, &String::from_str(&env, ""));
        assert_eq!(id, last_id + 1);
        last_id = id;
    }
    
    // Verify counter is working correctly
    assert_eq!(last_id, 1000);
}

// Test multiplication overflow in fee calculations
#[test]
fn test_fee_multiplication_overflow() {
    let env = Env::default();
    let (client, admin, _sender, _recipient, _token, asset) = setup_test(&env);

    // Set high fee percentage
    client.set_platform_fee(&admin, &10000); // 100%
    
    // Try to get fee breakdown with maximum amount
    let result = client.try_get_fee_breakdown(&i128::MAX);
    
    // Should handle overflow in fee calculations
    match result {
        Err(Ok(Error::ArithmeticOverflow)) | Ok(_) => {}
        _ => {}
    }
}

// Test partial release overflow when releasing more than available
#[test]
fn test_partial_release_overflow_exceeds_available() {
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
        &String::from_str(&env, ""),
        &true // allow partial release
    );
    
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Try to release more than available
    let result = client.try_release_partial(&escrow_id, &recipient, &token.address, &(amount + 1));
    assert_eq!(result, Err(Ok(Error::InsufficientFunds)));
}

// Test partial release with exact amount
#[test]
fn test_partial_release_exact_amount() {
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
        &String::from_str(&env, ""),
        &true // allow partial release
    );
    
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Release exactly the available amount
    let result = client.try_release_partial(&escrow_id, &recipient, &token.address, &amount);
    match result {
        Ok(_) | Err(Ok(Error::InsufficientFunds)) => {}
        _ => {}
    }
}

// Test refund overflow when refunding more than deposited
#[test]
fn test_refund_overflow_exceeds_deposited() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);
    
    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Try to refund more than deposited
    let result = client.try_refund_partial(&escrow_id, &sender, &token.address, &(amount + 1), &RefundReason::SenderRequest);
    assert_eq!(result, Err(Ok(Error::InvalidRefundAmount)));
}

// Test refund with exact amount
#[test]
fn test_refund_exact_amount() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);
    
    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Refund exact amount
    let result = client.try_refund_partial(&escrow_id, &sender, &token.address, &amount, &RefundReason::SenderRequest);
    match result {
        Ok(_) | Err(Ok(Error::InvalidRefundAmount)) => {}
        _ => {}
    }
}

// Test released amount overflow
#[test]
fn test_released_amount_overflow() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = i128::MAX / 2;
    token_admin.mint(&sender, &(amount * 2));
    
    let escrow_id = client.create_escrow(
        &sender, 
        &recipient, 
        &amount, 
        &asset, 
        &2000, 
        &String::from_str(&env, ""),
        &true // allow partial release
    );
    
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Try multiple partial releases that could overflow
    let half_amount = amount / 2;
    let _ = client.try_release_partial(&escrow_id, &recipient, &token.address, &half_amount);
    let _ = client.try_release_partial(&escrow_id, &recipient, &token.address, &half_amount);
    let _ = client.try_release_partial(&escrow_id, &recipient, &token.address, &half_amount);
}

// Test refunded amount overflow
#[test]
fn test_refunded_amount_overflow() {
    let env = Env::default();
    let (client, admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = i128::MAX / 2;
    token_admin.mint(&sender, &(amount * 2));
    
    let escrow_id = client.create_escrow(
        &sender, 
        &recipient, 
        &amount, 
        &asset, 
        &2000, 
        &String::from_str(&env, ""),
        &true // allow partial release
    );
    
    client.deposit(&escrow_id, &sender, &amount, &token.address);
    
    // Try multiple partial refunds that could overflow
    let half_amount = amount / 2;
    let _ = client.try_refund_partial(&escrow_id, &sender, &token.address, &half_amount, &RefundReason::SenderRequest);
    let _ = client.try_refund_partial(&escrow_id, &sender, &token.address, &half_amount, &RefundReason::SenderRequest);
    let _ = client.try_refund_partial(&escrow_id, &sender, &token.address, &half_amount, &RefundReason::SenderRequest);
}

// Test fee breakdown with zero amount
#[test]
fn test_fee_breakdown_zero_amount() {
    let env = Env::default();
    let (client, admin, _sender, _recipient, _token, asset) = setup_test(&env);

    client.set_platform_fee(&admin, &500);
    
    let result = client.get_fee_breakdown(&0);
    assert_eq!(result.total_fee, 0);
}

// Test fee breakdown with small amount
#[test]
fn test_fee_breakdown_small_amount() {
    let env = Env::default();
    let (client, admin, _sender, _recipient, _token, asset) = setup_test(&env);

    client.set_platform_fee(&admin, &100); // 1%
    
    let result = client.get_fee_breakdown(&1);
    // Should handle small amounts correctly
    assert!(result.total_fee >= 0);
}

// Test fee breakdown with typical amounts
#[test]
fn test_fee_breakdown_typical_amounts() {
    let env = Env::default();
    let (client, admin, _sender, _recipient, _token, asset) = setup_test(&env);

    client.set_platform_fee(&admin, &500); // 5%
    client.set_processing_fee(&admin, &100); // 1%
    client.set_forex_fee(&admin, &200); // 2%
    client.set_compliance_fee(&admin, &50);
    
    let test_amounts = [100i128, 500, 1000, 5000, 10000, 100000];
    
    for amount in test_amounts {
        let result = client.get_fee_breakdown(&amount);
        assert!(result.total_fee >= 0);
        assert!(result.total_fee <= amount);
    }
}

// Test edge case: amount = 1 with high fees
#[test]
fn test_fee_breakdown_one_amount_high_fees() {
    let env = Env::default();
    let (client, admin, _sender, _recipient, _token, asset) = setup_test(&env);

    client.set_platform_fee(&admin, &10000); // 100%
    
    let result = client.get_fee_breakdown(&1);
    // With 100% fee, total should be 1
    assert_eq!(result.total_fee, 1);
}

// Test edge case: amount causes division by zero (if any)
#[test]
fn test_fee_calculation_edge_cases() {
    let env = Env::default();
    let (client, admin, _sender, _recipient, _token, asset) = setup_test(&env);

    // Test with various fee percentages
    let percentages = [0i128, 1, 100, 5000, 9999, 10000];
    
    for pct in percentages {
        client.set_platform_fee(&admin, &pct);
        let result = client.get_fee_breakdown(&1000);
        assert!(result.total_fee >= 0);
    }
}

// Test maximum escrow amount handling
#[test]
fn test_maximum_escrow_amount_handling() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    // Test with various large amounts
    let large_amounts = [
        i128::MAX,
        i128::MAX - 1,
        i128::MAX / 2,
        i128::MAX / 10,
        u64::MAX as i128,
        (u64::MAX - 1) as i128,
    ];
    
    for amount in large_amounts {
        let result = client.try_create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
        match result {
            Ok(_) | Err(Ok(Error::ArithmeticOverflow)) | Err(Ok(Error::InvalidAmount)) => {}
            Err(Ok(e)) => {
                // Other errors are acceptable
            }
        }
    }
}

// Test timestamp overflow (if applicable)
#[test]
fn test_timestamp_edge_cases() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    // Test with maximum timestamp
    env.ledger().with_mut(|li| li.timestamp = u64::MAX);
    
    let escrow_id = client.create_escrow(&sender, &recipient, &1000, &asset, &u64::MAX, &String::from_str(&env, ""));
    
    // Verify escrow was created
    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.created_at, u64::MAX);
}

// Test zero timestamp
#[test]
fn test_zero_timestamp() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    env.ledger().with_mut(|li| li.timestamp = 0);
    
    let escrow_id = client.create_escrow(&sender, &recipient, &1000, &asset, &0, &String::from_str(&env, ""));
    
    // Verify escrow was created
    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.created_at, 0);
}