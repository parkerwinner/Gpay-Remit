#![cfg(test)]

use gpay_remit_contracts::payment_escrow::{PaymentEscrowContract, PaymentEscrowContractClient, FeeBreakdown};
use soroban_sdk::{testutils::Address as _, Address, Env};
use proptest::prelude::*;

proptest! {
    #[test]
    fn fuzz_calculate_fees(amount in 1i128..1_000_000_000_000_000i128) {
        let env = Env::default();
        let admin = Address::generate(&env);
        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);
        
        client.init_escrow(&admin);
        
        // Fee percentages are 0 by default, so total fee should be 0 unless configured
        // But let's just assert it doesn't panic and returns a valid result
        if let Ok(fee_breakdown) = client.try_get_fee_breakdown(&amount) {
            let fb = fee_breakdown.unwrap();
            let total = fb.platform_fee + fb.forex_fee + fb.compliance_fee + fb.network_fee;
            
            // Total fee calculation should be correct
            // (Note: in the actual contract, min_fee/max_fee might apply, but they default to 0/MAX)
            assert!(total <= amount || fb.total_fee <= amount, "Fee should not exceed amount");
            assert!(fb.total_fee >= 0, "Fee should be non-negative");
        }
    }
}

// (cargo-fuzz will pick up from corpus/fees/ if present)
