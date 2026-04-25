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
    EscrowCreated {
        escrow_id: u64,
        sender: Address,
        recipient: Address,
        asset: AssetRef,
        total_amount: i128,
    },
    EscrowDeposited {
        escrow_id: u64,
        amount: i128,
        deposited_total: i128,
    },
    EscrowApproved {
        escrow_id: u64,
    },
    EscrowReleased {
        escrow_id: u64,
        released_amount: i128,
    },
    EscrowRefunded {
        escrow_id: u64,
        refunded_amount: i128,
    },
    InvoiceCreated {
        invoice_id: u64,
        escrow_id: u64,
        sender: Address,
        recipient: Address,
        asset: AssetRef,
        total_due: i128,
    },
    InvoicePaid {
        invoice_id: u64,
        escrow_id: u64,
        paid_amount: i128,
    },
    InvoiceUpdated {
        invoice_id: u64,
        new_amount: i128,
        total_due: i128,
    },
    InvoiceCancelled {
        invoice_id: u64,
    },
    InvoiceOverdue {
        invoice_id: u64,
    },
    AdminAction {
        key: Symbol,
    },
    AddressAction {
        key: Symbol,
        address: Address,
    },
    PairAction {
        key: Symbol,
        first: Address,
        second: Address,
    },
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
