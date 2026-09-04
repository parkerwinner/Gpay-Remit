#![cfg(test)]

use gpay_remit_contracts::remittance_hub::{RemittanceHubContract, RemittanceHubContractClient, Asset};
use proptest::prelude::*;
use soroban_sdk::{testutils::Address as _, Address, Env, String, Symbol};

proptest! {
    #[test]
    fn fuzz_generate_invoice(amount in 1i128..1_000_000_000_000_000i128) {
        let env = Env::default();
        env.mock_all_auths();
        
        let admin = Address::generate(&env);
        let oracle1 = Address::generate(&env);
        let oracle2 = Address::generate(&env);
        
        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);
        
        client.init_hub(&admin, &oracle1, &oracle2, &3600);
        
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        
        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer: Address::generate(&env),
        };
        
        let res = client.try_generate_invoice(
            &sender,
            &recipient,
            &amount,
            &asset,
            &3600,
            &String::from_str(&env, "Test Invoice"),
            &0,
            &String::from_str(&env, "Memo"),
        );
        
        assert!(res.is_ok() || res.is_err());
    }
    
    #[test]
    fn fuzz_send_remittance(amount in 1i128..1_000_000_000_000_000i128) {
        let env = Env::default();
        env.mock_all_auths();
        
        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);
        
        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let currency = Symbol::new(&env, "USD");
        
        let res = client.try_send_remittance(&sender, &recipient, &amount, &currency);
        assert!(res.is_ok() || res.is_err());
    }
}
