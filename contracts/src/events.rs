use soroban_sdk::{contracttype, symbol_short, Address, Env, String, Symbol};

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub struct AssetRef {
    pub code: String,
    pub issuer: Address,
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
pub enum EventData {
    EscrowCreated,
    EscrowDeposited,
    EscrowApproved,
    EscrowReleased,
    EscrowRefunded,
    InvoiceCreated,
    InvoicePaid,
    InvoiceUpdated,
    InvoiceCancelled,
    InvoiceOverdue,
    AdminAction,
    AddressAction,
    PairAction,
}

#[cfg_attr(not(test), derive(Clone, Debug, PartialEq, Eq))]
#[cfg_attr(not(test), contracttype)]
#[cfg_attr(test, derive(Clone, Debug, PartialEq, Eq))]
pub struct GpayEvent {
    pub timestamp: u64,
    pub actor: Address,
    pub amount: i128,
    pub status: Symbol,
    pub data: EventData,
}

pub fn emit(
    env: &Env,
    component: Symbol,
    action: Symbol,
    id: u64,
    actor: &Address,
    amount: i128,
    status: Symbol,
    data: EventData,
) {
    #[cfg(not(test))]
    {
        env.events().publish(
            (symbol_short!("gpayremit"), component, action, id),
            GpayEvent {
                timestamp: env.ledger().timestamp(),
                actor: actor.clone(),
                amount,
                status,
                data,
            },
        );
    }
    #[cfg(test)]
    {
        // In test mode, publish a simpler event structure
        env.events().publish(
            (symbol_short!("gpayremit"), component, action, id),
            (env.ledger().timestamp(), actor.clone(), amount, status),
        );
    }
}
