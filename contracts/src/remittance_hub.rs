use soroban_sdk::{contract, contractimpl, contracttype, contracterror, Address, Env, Symbol, symbol_short};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum RemittanceError {
    InvalidAmount = 1,
    NotFound = 2,
    InvalidStatus = 3,
}

#[derive(Clone)]
#[contracttype]
pub struct RemittanceData {
    pub from: Address,
    pub to: Address,
    pub amount: i128,
    pub currency: Symbol,
    pub status: Symbol,
}

#[contract]
pub struct RemittanceHubContract;

#[contractimpl]
impl RemittanceHubContract {
    pub fn send_remittance(
        env: Env,
        from: Address,
        to: Address,
        amount: i128,
        currency: Symbol,
    ) -> Result<u64, RemittanceError> {
        from.require_auth();

        if amount <= 0 {
            return Err(RemittanceError::InvalidAmount);
        }

        let remittance_id = env.ledger().sequence() as u64;
        
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

    pub fn convert_currency(
        _env: Env,
        amount: i128,
        _from_currency: Symbol,
        _to_currency: Symbol,
    ) -> i128 {
        amount
    }

    pub fn complete_remittance(env: Env, remittance_id: u64, caller: Address) -> Result<(), RemittanceError> {
        caller.require_auth();

        let mut remittance: RemittanceData = env
            .storage()
            .persistent()
            .get(&remittance_id)
            .ok_or(RemittanceError::NotFound)?;

        if remittance.status != symbol_short!("pending") {
            return Err(RemittanceError::InvalidStatus);
        }

        remittance.status = symbol_short!("complete");
        env.storage().persistent().set(&remittance_id, &remittance);

        Ok(())
    }

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
        let remittance_id = client.send_remittance(&from, &to, &5000, &symbol_short!("USD"));

        let remittance = client.get_remittance(&remittance_id);
        assert!(remittance.is_some());
    }
}
