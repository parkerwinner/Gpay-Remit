use soroban_sdk::{contract, contractimpl, contracttype, contracterror, Address, Env, String, Symbol, symbol_short};

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq, PartialOrd, Ord)]
#[repr(u32)]
pub enum RemittanceError {
    InvalidAmount = 1,
    NotFound = 2,
    InvalidStatus = 3,
    DueDateInPast = 4,
    MissingEscrow = 5,
    InvoiceNotFound = 6,
    InvalidInvoiceStatus = 7,
    Unauthorized = 8,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
#[contracttype]
pub enum InvoiceStatus {
    Unpaid,
    Paid,
    Overdue,
    Cancelled,
}

#[derive(Clone)]
#[contracttype]
pub struct Asset {
    pub code: String,
    pub issuer: Address,
}

#[derive(Clone)]
#[contracttype]
pub struct Invoice {
    pub invoice_id: u64,
    pub sender: Address,
    pub recipient: Address,
    pub amount: i128,
    pub asset: Asset,
    pub converted_amount: i128,
    pub fees: i128,
    pub total_due: i128,
    pub status: InvoiceStatus,
    pub created_at: u64,
    pub due_date: u64,
    pub paid_at: u64,
    pub description: String,
    pub escrow_id: u64,
    pub memo: String,
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

#[derive(Clone, Copy)]
#[contracttype]
pub enum DataKey {
    InvoiceCounter,
    Invoice(u64),
    EscrowInvoice(u64),
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

    pub fn generate_invoice(
        env: Env,
        sender: Address,
        recipient: Address,
        amount: i128,
        asset: Asset,
        due_date: u64,
        description: String,
        escrow_id: u64,
        memo: String,
    ) -> Result<u64, RemittanceError> {
        sender.require_auth();

        if amount <= 0 {
            return Err(RemittanceError::InvalidAmount);
        }

        let current_time = env.ledger().timestamp();
        if due_date <= current_time {
            return Err(RemittanceError::DueDateInPast);
        }

        let mut counter: u64 = env.storage().persistent().get(&DataKey::InvoiceCounter).unwrap_or(0);
        counter = counter.checked_add(1).unwrap_or(counter);

        let exchange_rate = 10000;
        let converted_amount = amount.checked_mul(exchange_rate)
            .unwrap_or(amount)
            .checked_div(10000)
            .unwrap_or(amount);

        let fee_percentage = 250;
        let fees = amount.checked_mul(fee_percentage)
            .unwrap_or(0)
            .checked_div(10000)
            .unwrap_or(0);

        let total_due = amount.checked_add(fees).unwrap_or(amount);

        let invoice = Invoice {
            invoice_id: counter,
            sender: sender.clone(),
            recipient,
            amount,
            asset,
            converted_amount,
            fees,
            total_due,
            status: InvoiceStatus::Unpaid,
            created_at: current_time,
            due_date,
            paid_at: 0,
            description,
            escrow_id,
            memo,
        };

        env.storage().persistent().set(&DataKey::Invoice(counter), &invoice);
        env.storage().persistent().set(&DataKey::InvoiceCounter, &counter);

        if escrow_id > 0 {
            env.storage().persistent().set(&DataKey::EscrowInvoice(escrow_id), &counter);
        }

        env.events().publish(
            (symbol_short!("inv_gen"), counter),
            (sender, amount, total_due, due_date)
        );

        Ok(counter)
    }

    pub fn get_invoice(env: Env, invoice_id: u64) -> Option<Invoice> {
        env.storage().persistent().get(&DataKey::Invoice(invoice_id))
    }

    pub fn get_invoice_by_escrow(env: Env, escrow_id: u64) -> Option<u64> {
        env.storage().persistent().get(&DataKey::EscrowInvoice(escrow_id))
    }

    pub fn mark_invoice_paid(
        env: Env,
        invoice_id: u64,
        caller: Address,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();

        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        if invoice.status == InvoiceStatus::Paid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        if caller != invoice.sender && caller != invoice.recipient {
            return Err(RemittanceError::Unauthorized);
        }

        invoice.status = InvoiceStatus::Paid;
        invoice.paid_at = env.ledger().timestamp();

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_paid"), invoice_id),
            (caller, invoice.paid_at)
        );

        Ok(())
    }

    pub fn mark_invoice_overdue(
        env: Env,
        invoice_id: u64,
    ) -> Result<(), RemittanceError> {
        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        let current_time = env.ledger().timestamp();
        
        if current_time <= invoice.due_date {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        if invoice.status == InvoiceStatus::Paid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        invoice.status = InvoiceStatus::Overdue;

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_over"), invoice_id),
            current_time
        );

        Ok(())
    }

    pub fn cancel_invoice(
        env: Env,
        invoice_id: u64,
        caller: Address,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();

        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        if caller != invoice.sender {
            return Err(RemittanceError::Unauthorized);
        }

        if invoice.status == InvoiceStatus::Paid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        invoice.status = InvoiceStatus::Cancelled;

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_canc"), invoice_id),
            caller
        );

        Ok(())
    }

    pub fn update_invoice_amount(
        env: Env,
        invoice_id: u64,
        caller: Address,
        new_amount: i128,
    ) -> Result<(), RemittanceError> {
        caller.require_auth();

        if new_amount <= 0 {
            return Err(RemittanceError::InvalidAmount);
        }

        let mut invoice: Invoice = env.storage().persistent()
            .get(&DataKey::Invoice(invoice_id))
            .ok_or(RemittanceError::InvoiceNotFound)?;

        if caller != invoice.sender {
            return Err(RemittanceError::Unauthorized);
        }

        if invoice.status != InvoiceStatus::Unpaid {
            return Err(RemittanceError::InvalidInvoiceStatus);
        }

        let fee_percentage = 250;
        let fees = new_amount.checked_mul(fee_percentage)
            .unwrap_or(0)
            .checked_div(10000)
            .unwrap_or(0);

        invoice.amount = new_amount;
        invoice.fees = fees;
        invoice.total_due = new_amount.checked_add(fees).unwrap_or(new_amount);

        env.storage().persistent().set(&DataKey::Invoice(invoice_id), &invoice);

        env.events().publish(
            (symbol_short!("inv_upd"), invoice_id),
            (caller, new_amount, invoice.total_due)
        );

        Ok(())
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::testutils::{Address as _, Ledger};

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

    #[test]
    fn test_generate_invoice() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment for services"),
            &0,
            &String::from_str(&env, "Remittance memo")
        );

        assert_eq!(invoice_id, 1);

        let invoice = client.get_invoice(&invoice_id);
        assert!(invoice.is_some());

        let invoice_data = invoice.unwrap();
        assert_eq!(invoice_data.amount, 1000);
        assert_eq!(invoice_data.status, InvoiceStatus::Unpaid);
        assert_eq!(invoice_data.sender, sender);
        assert_eq!(invoice_data.recipient, recipient);
    }

    #[test]
    fn test_mark_invoice_paid() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        env.ledger().with_mut(|li| {
            li.timestamp = 1500;
        });

        client.mark_invoice_paid(&invoice_id, &sender);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.status, InvoiceStatus::Paid);
        assert_eq!(invoice.paid_at, 1500);
    }

    #[test]
    fn test_mark_invoice_overdue() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        env.ledger().with_mut(|li| {
            li.timestamp = 2500;
        });

        client.mark_invoice_overdue(&invoice_id);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.status, InvoiceStatus::Overdue);
    }

    #[test]
    fn test_cancel_invoice() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        client.cancel_invoice(&invoice_id, &sender);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.status, InvoiceStatus::Cancelled);
    }

    #[test]
    fn test_update_invoice_amount() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        client.update_invoice_amount(&invoice_id, &sender, &1500);

        let invoice = client.get_invoice(&invoice_id).unwrap();
        assert_eq!(invoice.amount, 1500);
        let expected_fee = 1500 * 250 / 10000;
        assert_eq!(invoice.fees, expected_fee);
        assert_eq!(invoice.total_due, 1500 + expected_fee);
    }

    #[test]
    fn test_invoice_with_escrow_link() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 1000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let escrow_id = 123;
        let invoice_id = client.generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &2000,
            &String::from_str(&env, "Payment"),
            &escrow_id,
            &String::from_str(&env, "Memo")
        );

        let linked_invoice_id = client.get_invoice_by_escrow(&escrow_id);
        assert!(linked_invoice_id.is_some());
        assert_eq!(linked_invoice_id.unwrap(), invoice_id);
    }

    #[test]
    fn test_invoice_due_date_validation() {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| {
            li.timestamp = 2000;
        });

        let contract_id = env.register_contract(None, RemittanceHubContract);
        let client = RemittanceHubContractClient::new(&env, &contract_id);

        let sender = Address::generate(&env);
        let recipient = Address::generate(&env);
        let issuer = Address::generate(&env);

        let asset = Asset {
            code: String::from_str(&env, "USDC"),
            issuer,
        };

        let result = client.try_generate_invoice(
            &sender,
            &recipient,
            &1000,
            &asset,
            &1500,
            &String::from_str(&env, "Payment"),
            &0,
            &String::from_str(&env, "Memo")
        );

        assert_eq!(result, Err(Ok(RemittanceError::DueDateInPast)));
    }
}
