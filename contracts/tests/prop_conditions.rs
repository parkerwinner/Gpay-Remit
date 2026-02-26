#![cfg(all(test, feature = "prop"))]

use gpay_remit_contracts::payment_escrow::{
    Asset, ConditionOperator, ConditionType, PaymentEscrowContract, PaymentEscrowContractClient,
};
use proptest::prelude::*;
use soroban_sdk::{
    testutils::{Address as _, Ledger},
    Address, Env, String,
};

// --- Helpers ---

fn setup_escrow_env(env: &Env) -> (PaymentEscrowContractClient, Address, Asset) {
    let contract_id = env.register_contract(None, PaymentEscrowContract);
    let client = PaymentEscrowContractClient::new(env, &contract_id);
    let admin = Address::generate(env);
    client.init_escrow(&admin);

    let asset = Asset {
        code: String::from_str(env, "USDC"),
        issuer: Address::generate(env),
    };
    client.add_supported_asset(&admin, &asset);

    (client, admin, asset)
}

fn create_base_escrow(
    env: &Env,
    client: &PaymentEscrowContractClient,
    _admin: &Address,
    asset: &Asset,
    expiration: u64,
) -> u64 {
    client.create_escrow(
        &Address::generate(env),
        &Address::generate(env),
        &1000,
        asset,
        &expiration,
        &String::from_str(env, ""),
    )
}

// --- Generators ---

fn arb_condition_operator() -> impl Strategy<Value = ConditionOperator> {
    prop_oneof![Just(ConditionOperator::And), Just(ConditionOperator::Or)]
}

fn arb_condition_type() -> impl Strategy<Value = ConditionType> {
    prop_oneof![
        Just(ConditionType::Timestamp),
        Just(ConditionType::Approval),
        Just(ConditionType::OraclePrice),
        Just(ConditionType::MultiSignature),
        Just(ConditionType::KYCVerified),
    ]
}

#[derive(Debug, Clone)]
struct ArbCondition {
    condition_type: ConditionType,
    required: bool,
    threshold_value: i128,
}

fn arb_condition() -> impl Strategy<Value = ArbCondition> {
    (arb_condition_type(), any::<bool>(), -100i128..2000i128).prop_map(
        |(condition_type, required, threshold_value)| ArbCondition {
            condition_type,
            required,
            threshold_value,
        },
    )
}

// --- Tests ---

proptest! {
    #![proptest_config(ProptestConfig::with_cases(1000))]

    #[test]
    fn test_verify_idempotency(
        expiration in any::<u64>(),
        ledger_time in any::<u64>(),
        proof_data in any::<i128>(),
        operator in arb_condition_operator()
    ) {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| li.timestamp = ledger_time);

        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &admin, &asset, expiration);

        client.set_condition_operator(&escrow_id, &admin, &operator);

        let result1 = client.verify_conditions(&escrow_id, &proof_data);
        let result2 = client.verify_conditions(&escrow_id, &proof_data);

        assert_eq!(result1.all_passed, result2.all_passed);
        assert_eq!(result1.failed_conditions.len(), result2.failed_conditions.len());
    }

    #[test]
    fn test_monotonicity_timestamp(
        initial_time in 0u64..1000000000u64,
        delta in 0u64..1000000000u64,
        expiration in 0u64..2000000000u64,
    ) {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &admin, &asset, expiration);

        // Add timestamp condition
        client.set_min_approvals(&escrow_id, &admin, &0);
        client.add_condition(&escrow_id, &admin, &ConditionType::Timestamp, &true, &0);

        // Test at T
        env.ledger().with_mut(|li| li.timestamp = initial_time);
        let res_t = client.verify_conditions(&escrow_id, &0).all_passed;

        // Test at T + Delta
        env.ledger().with_mut(|li| li.timestamp = initial_time + delta);
        let res_t_plus = client.verify_conditions(&escrow_id, &0).all_passed;

        if res_t {
            assert!(res_t_plus, "If timestamp condition passed at T, it should pass at T+Delta");
        }
    }

    #[test]
    fn test_composability_and_operator(
        num_conds in 1usize..5,
        thresholds in prop::collection::vec(0i128..1000i128, 1..5),
        proof in 0i128..2000i128
    ) {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &admin, &asset, 0);

        client.set_condition_operator(&escrow_id, &admin, &ConditionOperator::And);
        client.set_min_approvals(&escrow_id, &admin, &0);

        let mut expected_pass = true;
        for i in 0..num_conds.min(thresholds.len()) {
            let threshold = thresholds[i];
            client.add_condition(&escrow_id, &admin, &ConditionType::OraclePrice, &true, &threshold);
            if proof < threshold {
                expected_pass = false;
            }
        }

        let result = client.verify_conditions(&escrow_id, &proof);
        assert_eq!(result.all_passed, expected_pass, "AND operator failed: proof={}, thresholds={:?}", proof, thresholds);
    }

    #[test]
    fn test_empty_conditions(
        operator in arb_condition_operator()
    ) {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &admin, &asset, 0);

        client.set_condition_operator(&escrow_id, &admin, &operator);

        let result = client.verify_conditions(&escrow_id, &0);

        // If there are no conditions, AND should pass (identity for AND is true, but check code)
        // Code says: failed_conditions.is_empty() && (required_count == 0 || passed_count >= required_count)
        // for OR: passed_count > 0
        match operator {
            ConditionOperator::And => assert!(result.all_passed),
            ConditionOperator::Or => assert!(!result.all_passed),
        }
    }

    #[test]
    fn test_timestamp_extremes(
        expiration in prop_oneof![Just(0u64), Just(u64::MAX), any::<u64>()],
        ledger_time in prop_oneof![Just(0u64), Just(u64::MAX), any::<u64>()],
    ) {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| li.timestamp = ledger_time);

        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &admin, &asset, expiration);

        client.set_min_approvals(&escrow_id, &admin, &0);
        client.add_condition(&escrow_id, &admin, &ConditionType::Timestamp, &true, &0);

        let result = client.verify_conditions(&escrow_id, &0);

        if ledger_time >= expiration {
            assert!(result.all_passed, "Should pass when ledger_time ({}) >= expiration ({})", ledger_time, expiration);
        } else {
            assert!(!result.all_passed, "Should fail when ledger_time ({}) < expiration ({})", ledger_time, expiration);
        }
    }

    #[test]
    fn test_mixed_composability(
        conds in prop::collection::vec(arb_condition(), 1..10),
        operator in arb_condition_operator(),
        proof in -100i128..2000i128,
        ledger_time in any::<u64>(),
        kyc_compliant in any::<bool>(),
        approvals in 0u32..10u32,
        min_approvals in 0u32..10u32,
    ) {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| li.timestamp = ledger_time);

        let (client, admin, asset) = setup_escrow_env(&env);
        // Expiration for timestamp conditions is set during create_base_escrow
        // But the ConditionType::Timestamp check in verify_conditions uses escrow.release_conditions.expiration_timestamp
        let expiration = ledger_time.saturating_sub(100); // Make it pass by default if we want
        let escrow_id = create_base_escrow(&env, &client, &admin, &asset, expiration);

        if kyc_compliant {
            client.admin_override_kyc(&admin, &escrow_id);
        }

        // Set min_approvals and current_approvals (via loop)
        // Since we can't set them directly easily, we call add_approval
        client.set_min_approvals(&escrow_id, &admin, &min_approvals);
        for _ in 0..approvals {
            client.add_approval(&escrow_id, &admin);
        }

        // Wait, we need to know what min_approvals is.
        // add_condition for Approval/MultiSignature doesn't set min_approvals in the contract call.
        // It seems min_approvals is set during create_escrow? No, create_escrow uses 0 by default.
        // Let's check create_escrow implementation.

        client.set_condition_operator(&escrow_id, &admin, &operator);

        let mut expected_passed_count = 0;
        let mut expected_required_count = 0;
        let mut expected_failed_required = false;

        for arb_c in conds.iter() {
            client.add_condition(&escrow_id, &admin, &arb_c.condition_type, &arb_c.required, &arb_c.threshold_value);
            if arb_c.required {
                expected_required_count += 1;
            }

            let is_passed = match arb_c.condition_type {
                ConditionType::Timestamp => ledger_time >= expiration,
                ConditionType::Approval | ConditionType::MultiSignature => {
                    approvals >= min_approvals
                }
                ConditionType::OraclePrice => {
                    if proof > 0 { proof >= arb_c.threshold_value } else { false }
                }
                ConditionType::KYCVerified => kyc_compliant,
            };

            if is_passed {
                expected_passed_count += 1;
            } else if arb_c.required {
                expected_failed_required = true;
            }
        }

        let result = client.verify_conditions(&escrow_id, &proof);

        let expected_all_passed = match operator {
            ConditionOperator::And => !expected_failed_required && (expected_required_count == 0 || expected_passed_count >= expected_required_count),
            ConditionOperator::Or => expected_passed_count > 0,
        };

        assert_eq!(result.all_passed, expected_all_passed,
            "Operator: {:?}, Expected pass: {}, Got: {}, Proof: {}, KYC: {}, Approvals: {}, Ledger: {}, Exp: {}",
            operator, expected_all_passed, result.all_passed, proof, kyc_compliant, approvals, ledger_time, expiration);
    }
}
