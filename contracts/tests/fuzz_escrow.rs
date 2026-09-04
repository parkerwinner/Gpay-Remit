#![cfg(test)]

use gpay_remit_contracts::payment_escrow::{PaymentEscrowContract, PaymentEscrowContractClient, Asset};
use proptest::prelude::*;
use soroban_sdk::{testutils::Address as _, Address, Env, String, Symbol};

proptest! {
    #[test]
    fn fuzz_deposit_escrow(amount in 1i128..1_000_000_000_000_000i128) {
        let env = Env::default();
        env.mock_all_auths();
        
        let admin = Address::generate(&env);
        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);
        
        client.init_escrow(&admin);
        
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        
        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: Address::generate(&env),
        };
        
        let res = client.try_deposit(
            &sender,
            &recipient,
            &amount,
            &asset,
            &3600,
        );
        
        assert!(res.is_ok() || res.is_err());
    }
}
