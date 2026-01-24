use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol, symbol_short};

#[derive(Clone)]
#[contracttype]
pub struct RemittanceData {
    pub from: Address,
    pub to: Address,
    pub amount: i128,
    pub currency: Symbol,
    pub status: Symbol, // pending, completed, failed
}

#[contract]
pub struct RemittanceHubContract;

#[contractimpl]
impl RemittanceHubContract {
    /// Initiate a remittance transfer
    pub fn send_remittance(
        env: Env,
        from: Address,
        to: Address,
        amount: i128,
        currency: Symbol,
    ) -> Result<u64, Symbol> {
        from.require_auth();

        if amount <= 0 {
            return Err(symbol_short!("inv_amt"));
        }

        let remittance_id = env.ledger().sequence();
        
        let remittance = RemittanceData {
            from: from.clone(),
            to,
            amount,
            currency,
            status: symbol_short!("pending"),
        };

        env.storage()
            .persistent()
            .set(&remittance_id, &remittance);

        Ok(remittance_id)
    }

    /// Convert currency using Stellar DEX (stub for oracle integration)
    pub fn convert_currency(
        _env: Env,
        amount: i128,
        _from_currency: Symbol,
        _to_currency: Symbol,
    ) -> i128 {
        amount
    }

    /// Complete remittance transfer
    pub fn complete_remittance(env: Env, remittance_id: u64, caller: Address) -> Result<(), Symbol> {
        caller.require_auth();

        let mut remittance: RemittanceData = env
            .storage()
            .persistent()
            .get(&remittance_id)
            .ok_or(symbol_short!("not_fnd"))?;

        if remittance.status != symbol_short!("pending") {
            return Err(symbol_short!("inv_stat"));
        }

        remittance.status = symbol_short!("complete");
        env.storage().persistent().set(&remittance_id, &remittance);

        Ok(())
    }

    /// Get remittance details
    pub fn get_remittance(env: Env, remittance_id: u64) -> Option<RemittanceData> {
        env.storage().persistent().get(&remittance_id)
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::Address as _;

    #[test]
    fn test_send_remittance() {
        let env = Env::default();
        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let from = Address::generate(&env);
        let to = Address::generate(&env);

        env.mock_all_auths();
        let result = client.send_remittance(&from, &to, &5000, &symbol_short!("USD"));
        assert!(result.is_ok());

        let remittance_id = result.unwrap();
        let remittance = client.get_remittance(&remittance_id);
        assert!(remittance.is_some());
    }
}
