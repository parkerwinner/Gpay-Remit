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
    EscrowCreated(u64, Address, Address, AssetRef, i128),
    EscrowDeposited(u64, i128, i128),
    EscrowApproved(u64),
    EscrowReleased(u64, i128),
    EscrowRefunded(u64, i128),
    InvoiceCreated(u64, u64, Address, Address, AssetRef, i128),
    InvoicePaid(u64, u64, i128),
    InvoiceUpdated(u64, i128, i128),
    InvoiceCancelled(u64),
    InvoiceOverdue(u64),
    AdminAction(Symbol),
    AddressAction(Symbol, Address),
    PairAction(Symbol, Address, Address),
}

#[derive(Clone, Debug, PartialEq, Eq)]
#[contracttype]
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
