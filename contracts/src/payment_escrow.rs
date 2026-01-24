use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol, symbol_short};

#[derive(Clone)]
#[contracttype]
pub struct EscrowData {
    pub sender: Address,
    pub recipient: Address,
    pub amount: i128,
    pub released: bool,
}

#[contract]
pub struct PaymentEscrowContract;

#[contractimpl]
impl PaymentEscrowContract {
    /// Initialize escrow with sender, recipient, and amount
    pub fn deposit(
        env: Env,
        sender: Address,
        recipient: Address,
        amount: i128,
    ) -> Result<(), Symbol> {
        sender.require_auth();

        if amount <= 0 {
            return Err(symbol_short!("inv_amt"));
        }

        let escrow = EscrowData {
            sender: sender.clone(),
            recipient,
            amount,
            released: false,
        };

        env.storage().instance().set(&symbol_short!("escrow"), &escrow);
        Ok(())
    }

    /// Release funds to recipient (can add conditions here)
    pub fn release(env: Env, caller: Address) -> Result<(), Symbol> {
        caller.require_auth();

        let mut escrow: EscrowData = env
            .storage()
            .instance()
            .get(&symbol_short!("escrow"))
            .ok_or(symbol_short!("no_escr"))?;

        if escrow.released {
            return Err(symbol_short!("released"));
        }

        escrow.released = true;
        env.storage().instance().set(&symbol_short!("escrow"), &escrow);

        Ok(())
    }

    /// Get escrow details
    pub fn get_escrow(env: Env) -> Option<EscrowData> {
        env.storage().instance().get(&symbol_short!("escrow"))
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::Address as _;

    #[test]
    fn test_deposit() {
        let env = Env::default();
        let contract_id = env.register_contract(None, PaymentEscrowContract);
        let client = PaymentEscrowContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);

        env.mock_all_auths();
        let result = client.deposit(&sender, &recipient, &1000);
        assert!(result.is_ok());

        let escrow = client.get_escrow();
        assert!(escrow.is_some());
        assert_eq!(escrow.unwrap().amount, 1000);
    }
}
