//! End-to-end test that deploys BOTH contracts (payment escrow + remittance hub)
//! together with the mock KYC and AML oracles, and drives the full lifecycle:
//!
//!   create escrow (KYC check) -> deposit -> hub invoice linked to the escrow ->
//!   hub remittance (AML screen) -> release escrow (fees to treasury) ->
//!   invoice paid -> remittance completed
//!
//! It also pins the fee vectors that `backend/services/fees_test.go` asserts, so
//! the on-chain and off-chain fee calculations cannot drift apart (#377).

use gpay_remit_contracts::aml::{AmlStatus, MockAmlOracleContract, MockAmlOracleContractClient};
use gpay_remit_contracts::events::{EventData, GpayEvent};
use gpay_remit_contracts::kyc::{MockKycOracleContract, MockKycOracleContractClient};
use gpay_remit_contracts::payment_escrow::{
    Asset as EscrowAsset, EscrowStatus, PaymentEscrowContract, PaymentEscrowContractClient,
};
use gpay_remit_contracts::remittance_hub::{
    Asset as HubAsset, FeeConfig, InvoiceStatus, RemittanceError, RemittanceHubContract,
    RemittanceHubContractClient,
};
use soroban_sdk::{
    symbol_short,
    testutils::{Address as _, Events, Ledger},
    token, Address, Env, IntoVal, String, Symbol,
};

struct World<'a> {
    env: Env,
    escrow: PaymentEscrowContractClient<'a>,
    hub: RemittanceHubContractClient<'a>,
    token: token::Client<'a>,
    admin: Address,
    sender: Address,
    recipient: Address,
    treasury: Address,
    aml: MockAmlOracleContractClient<'a>,
    kyc: MockKycOracleContractClient<'a>,
    escrow_asset: EscrowAsset,
    hub_asset: HubAsset,
}

const AMOUNT: i128 = 100_000;
/// 2.5% platform + 1% forex on the escrow ...
const ESCROW_PLATFORM_BPS: i128 = 250;
const ESCROW_FOREX_BPS: i128 = 100;
/// ... and the hub is configured with the same 3.5% total.
const HUB_TOTAL_BPS: i128 = 350;

fn world<'a>() -> World<'a> {
    let env = Env::default();
    env.mock_all_auths();
    env.ledger().with_mut(|li| li.timestamp = 1_000);

    let admin = Address::generate(&env);
    let sender = Address::generate(&env);
    let recipient = Address::generate(&env);
    let treasury = Address::generate(&env);

    // Token
    let sac = env.register_stellar_asset_contract_v2(admin.clone());
    let token = token::Client::new(&env, &sac.address());
    token::StellarAssetClient::new(&env, &sac.address()).mint(&sender, &(AMOUNT * 10));

    // Oracles
    let kyc_id = env.register_contract(None, MockKycOracleContract);
    let kyc = MockKycOracleContractClient::new(&env, &kyc_id);
    kyc.init_kyc(&admin);
    kyc.set_status(&admin, &sender, &1);
    kyc.set_status(&admin, &recipient, &1);

    let aml_id = env.register_contract(None, MockAmlOracleContract);
    let aml = MockAmlOracleContractClient::new(&env, &aml_id);
    aml.initialize(&admin);

    // Escrow contract
    let escrow_id = env.register_contract(None, PaymentEscrowContract);
    let escrow = PaymentEscrowContractClient::new(&env, &escrow_id);
    escrow.init_escrow(&admin);
    let escrow_asset = EscrowAsset {
        code: String::from_str(&env, "USDC"),
        issuer: admin.clone(),
    };
    escrow.add_supported_asset(&admin, &escrow_asset);
    escrow.configure_kyc(&admin, &kyc_id, &true, &0);
    escrow.set_platform_fee(&admin, &ESCROW_PLATFORM_BPS);
    escrow.set_forex_fee(&admin, &ESCROW_FOREX_BPS);
    escrow.set_fee_wallet(&admin, &treasury);

    // Hub contract
    let hub_id = env.register_contract(None, RemittanceHubContract);
    let hub = RemittanceHubContractClient::new(&env, &hub_id);
    hub.init_hub(&admin, &admin, &admin, &3600);
    hub.set_fee_config(
        &admin,
        &FeeConfig {
            percentage: HUB_TOTAL_BPS,
            min_fee: 0,
            max_fee: i128::MAX,
        },
    );
    hub.set_treasury(&admin, &treasury);
    hub.configure_aml(&admin, &aml_id, &70);
    let hub_asset = HubAsset {
        code: String::from_str(&env, "USDC"),
        issuer: admin.clone(),
    };

    World {
        env,
        escrow,
        hub,
        token,
        admin,
        sender,
        recipient,
        treasury,
        aml,
        kyc,
        escrow_asset,
        hub_asset,
    }
}

#[test]
fn full_escrow_to_remittance_lifecycle() {
    let w = world();
    let env = &w.env;
    let usd = Symbol::new(env, "USD");

    // 1. Create escrow: KYC is enforced by the escrow contract.
    let escrow_id = w.escrow.create_escrow(
        &w.sender,
        &w.recipient,
        &AMOUNT,
        &w.escrow_asset,
        &5_000,
        &String::from_str(env, "invoice 1"),
    );
    let escrow = w.escrow.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Pending);
    assert!(escrow.kyc_compliant, "KYC oracle verified both parties");

    // 2. Deposit funds.
    w.escrow
        .deposit(&escrow_id, &w.sender, &AMOUNT, &w.token.address);
    let escrow = w.escrow.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Funded);
    assert_eq!(escrow.deposited_amount, AMOUNT);
    assert_eq!(w.token.balance(&w.escrow.address), AMOUNT);

    // 3. Hub invoice linked to the escrow; the hub quotes the same fee.
    let invoice_id = w.hub.generate_invoice(
        &w.sender,
        &w.recipient,
        &AMOUNT,
        &w.hub_asset,
        &9_000,
        &String::from_str(env, "invoice 1"),
        &escrow_id,
        &String::from_str(env, "memo"),
    );
    assert_eq!(w.hub.get_invoice_by_escrow(&escrow_id), Some(invoice_id));
    let invoice = w.hub.get_invoice(&invoice_id).unwrap();
    assert_eq!(invoice.status, InvoiceStatus::Unpaid);
    let expected_fee = AMOUNT * (ESCROW_PLATFORM_BPS + ESCROW_FOREX_BPS) / 10_000;
    assert_eq!(invoice.fees, expected_fee);
    assert_eq!(invoice.total_due, AMOUNT + expected_fee);

    // 4. Hub remittance passes AML screening (score 0 < threshold 70).
    let remittance_id = w
        .hub
        .send_remittance(&w.sender, &w.recipient, &AMOUNT, &usd);
    let remittance = w.hub.get_remittance(&remittance_id).unwrap();
    assert_eq!(remittance.status, symbol_short!("pending"));
    assert!(w.hub.get_aml_flag(&remittance_id).is_none());

    // 5. Release the escrow: recipient gets amount - fee, treasury gets the fee.
    w.escrow
        .release_escrow(&escrow_id, &w.recipient, &w.token.address);
    let released_event = (
        w.escrow.address.clone(),
        (
            symbol_short!("gpayremit"),
            symbol_short!("escrow"),
            symbol_short!("released"),
            escrow_id,
        )
            .into_val(env),
        GpayEvent {
            timestamp: env.ledger().timestamp(),
            ledger: env.ledger().sequence(),
            actor: w.recipient.clone(),
            amount: AMOUNT - expected_fee,
            status: symbol_short!("released"),
            data: EventData::EscrowReleased(escrow_id, AMOUNT - expected_fee),
        }
        .into_val(env),
    );
    assert!(
        env.events().all().contains(&released_event),
        "escrow must emit a `released` event"
    );
    let escrow = w.escrow.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.status, EscrowStatus::Released);
    assert_eq!(escrow.released_amount, AMOUNT);
    assert_eq!(w.token.balance(&w.recipient), AMOUNT - expected_fee);
    assert_eq!(w.token.balance(&w.treasury), expected_fee);
    assert_eq!(w.token.balance(&w.escrow.address), 0);
    assert_eq!(
        w.token.balance(&w.admin),
        0,
        "fees go to the treasury, not the admin"
    );

    // 6. Invoice is paid and the remittance completes.
    w.hub.mark_invoice_paid(&invoice_id, &w.recipient);
    let paid_event = (
        w.hub.address.clone(),
        (
            symbol_short!("gpayremit"),
            symbol_short!("hub"),
            symbol_short!("inv_paid"),
            invoice_id,
        )
            .into_val(env),
        GpayEvent {
            timestamp: env.ledger().timestamp(),
            ledger: env.ledger().sequence(),
            actor: w.recipient.clone(),
            amount: invoice.total_due,
            status: symbol_short!("paid"),
            data: EventData::InvoicePaid(invoice_id, escrow_id, invoice.total_due),
        }
        .into_val(env),
    );
    assert!(
        env.events().all().contains(&paid_event),
        "hub must emit an `inv_paid` event"
    );
    let invoice = w.hub.get_invoice(&invoice_id).unwrap();
    assert_eq!(invoice.status, InvoiceStatus::Paid);
    assert_eq!(invoice.escrow_id, escrow_id);

    w.hub.complete_remittance(&remittance_id, &w.sender);
    let remittance = w.hub.get_remittance(&remittance_id).unwrap();
    assert_eq!(remittance.status, symbol_short!("complete"));
}

#[test]
fn kyc_failure_blocks_escrow_creation() {
    let w = world();
    // Revoke the recipient's KYC at the oracle.
    w.kyc.set_status(&w.admin, &w.recipient, &0);

    let res = w.escrow.try_create_escrow(
        &w.sender,
        &w.recipient,
        &AMOUNT,
        &w.escrow_asset,
        &5_000,
        &String::from_str(&w.env, "no kyc"),
    );
    assert!(res.is_err(), "escrow must not be created without KYC");
}

#[test]
fn aml_flagged_remittance_cannot_complete_but_escrow_unaffected() {
    let w = world();
    let usd = Symbol::new(&w.env, "USD");
    w.aml.set_risk_score(&w.admin, &w.recipient, &95);

    let id = w
        .hub
        .send_remittance(&w.sender, &w.recipient, &AMOUNT, &usd);
    assert_eq!(
        w.hub.get_remittance(&id).unwrap().status,
        symbol_short!("flagged")
    );
    assert_eq!(w.hub.get_aml_flag(&id).unwrap().status, AmlStatus::Flagged);
    assert_eq!(
        w.hub.try_complete_remittance(&id, &w.sender),
        Err(Ok(RemittanceError::AmlHighRisk))
    );

    // Compliance clears the flag -> remittance completes.
    w.hub.clear_aml_flag(&w.admin, &id);
    w.hub.complete_remittance(&id, &w.sender);
    assert_eq!(
        w.hub.get_remittance(&id).unwrap().status,
        symbol_short!("complete")
    );
}

#[test]
fn hub_batch_release_forwards_collected_fee_to_treasury() {
    let w = world();
    let env = &w.env;

    let ids = w.hub.batch_create_escrows(
        &w.sender,
        &soroban_sdk::vec![
            env,
            gpay_remit_contracts::remittance_hub::EscrowRequest {
                recipient: w.recipient.clone(),
                amount: AMOUNT,
                asset: w.hub_asset.clone(),
                expiration_timestamp: 5_000,
            }
        ],
    );
    w.hub.batch_deposit(&w.sender, &ids, &w.token.address);
    let fee = w.hub.get_fee(&AMOUNT);
    assert_eq!(fee, AMOUNT * HUB_TOTAL_BPS / 10_000);
    // Sender paid amount + fee into the hub
    assert_eq!(w.token.balance(&w.hub.address), AMOUNT + fee);

    // Changing the rate after deposit must not change what the treasury gets.
    w.hub.set_fee_config(
        &w.admin,
        &FeeConfig {
            percentage: 1_000,
            min_fee: 0,
            max_fee: i128::MAX,
        },
    );
    w.hub.batch_release(&w.recipient, &ids, &w.token.address);

    assert_eq!(w.token.balance(&w.recipient), AMOUNT);
    assert_eq!(w.token.balance(&w.treasury), fee);
    assert_eq!(w.token.balance(&w.hub.address), 0);
}

#[test]
fn hub_fee_config_is_admin_only_and_treasury_is_admin_only() {
    let w = world();
    let cfg = FeeConfig {
        percentage: 100,
        min_fee: 0,
        max_fee: 1_000,
    };
    assert_eq!(
        w.hub.try_set_fee_config(&w.sender, &cfg),
        Err(Ok(RemittanceError::Unauthorized))
    );
    assert_eq!(
        w.hub.try_set_treasury(&w.sender, &w.sender),
        Err(Ok(RemittanceError::Unauthorized))
    );
    assert_eq!(w.hub.get_treasury(), Some(w.treasury.clone()));
}

#[test]
fn hub_fee_does_not_vanish_on_overflow() {
    let w = world();
    // i128::MAX * 350 overflows; the fee must saturate to max_fee, not become 0.
    w.hub.set_fee_config(
        &w.admin,
        &FeeConfig {
            percentage: 10_000,
            min_fee: 0,
            max_fee: 5_000,
        },
    );
    assert_eq!(w.hub.get_fee(&i128::MAX), 5_000);
}

/// Shared with `backend/services/fees_test.go` (TestFeeParityWithContract).
/// (amount, platform_bps, min_fee, max_fee, expected_fee)
const FEE_VECTORS: [(i128, i128, i128, i128, i128); 6] = [
    (10_000, 250, 0, i128::MAX, 250),
    (1_000, 10, 100, 1_000, 100),
    (1_000_000, 500, 100, 1_000, 1_000),
    (999, 250, 0, i128::MAX, 24),
    (1, 250, 0, i128::MAX, 0),
    (123_456_789, 175, 0, i128::MAX, 2_160_493),
];

#[test]
fn onchain_fees_match_offchain_vectors() {
    for (amount, bps, min_fee, max_fee, expected) in FEE_VECTORS {
        let w = world();
        // Escrow: platform fee only
        w.escrow.set_forex_fee(&w.admin, &0);
        w.escrow.set_platform_fee(&w.admin, &bps);
        w.escrow.set_fee_limits(&w.admin, &min_fee, &max_fee);
        assert_eq!(
            w.escrow.get_fee_breakdown(&amount).total_fee,
            expected,
            "escrow fee for amount={amount} bps={bps}"
        );

        // Hub
        w.hub.set_fee_config(
            &w.admin,
            &FeeConfig {
                percentage: bps,
                min_fee,
                max_fee,
            },
        );
        assert_eq!(
            w.hub.get_fee(&amount),
            expected,
            "hub fee for amount={amount} bps={bps}"
        );
    }
}
