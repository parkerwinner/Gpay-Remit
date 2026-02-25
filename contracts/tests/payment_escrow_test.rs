use gpay_remit_contracts::payment_escrow::{PaymentEscrowContract, PaymentEscrowContractClient, Asset, EscrowStatus, Error};
use soroban_sdk::{testutils::{Address as _, Ledger, Events as _}, token, Address, Env, String, symbol_short, Symbol, FromVal};

fn create_token_contract<'a>(
    env: &Env,
    admin: &Address,
) -> (token::Client<'a>, token::StellarAssetClient<'a>) {
    let contract_address = env.register_stellar_asset_contract_v2(admin.clone());
    (
        token::Client::new(env, &contract_address.address()),
        token::StellarAssetClient::new(env, &contract_address.address()),
    )
}

fn setup_test<'a>(env: &Env) -> (PaymentEscrowContractClient<'a>, Address, Address, Address, (token::Client<'a>, token::StellarAssetClient<'a>), Asset) {
    env.mock_all_auths();
    let contract_id = env.register_contract(None, PaymentEscrowContract);
    let client = PaymentEscrowContractClient::new(env, &contract_id);

    let admin = Address::generate(env);
    let sender = Address::generate(env);
    let recipient = Address::generate(env);

    client.init_escrow(&admin);

    let token = create_token_contract(env, &admin);
    let asset = Asset {
        code: String::from_str(env, "USDC"),
        issuer: admin.clone(),
    };

    client.add_supported_asset(&admin, &asset);

    (client, admin, sender, recipient, token, asset)
}

#[test]
fn test_create_escrow_success() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    env.ledger().with_mut(|li| li.timestamp = 1000);

    let memo = String::from_str(&env, "Test Memo");
    let expiration = 2000;
    let amount = 1000;

    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &expiration, &memo);
    assert_eq!(escrow_id, 1);

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.sender, sender);
    assert_eq!(escrow.recipient, recipient);
    assert_eq!(escrow.amount, amount);
    assert_eq!(escrow.asset.code, asset.code);
    assert_eq!(escrow.status, EscrowStatus::Pending);
    assert_eq!(escrow.created_at, 1000);
}

#[test]
fn test_deposit_success() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    env.ledger().with_mut(|li| li.timestamp = 1000);
    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
    
    client.deposit(&escrow_id, &sender, &amount, &token.address);

    let escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.deposited_amount, amount);
    assert_eq!(escrow.status, EscrowStatus::Funded);
    assert_eq!(token.balance(&client.address), amount);
}

#[test]
fn test_partial_deposit_success() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));

    client.deposit(&escrow_id, &sender, &400, &token.address);
    let mut escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.deposited_amount, 400);
    assert_eq!(escrow.status, EscrowStatus::Pending);

    client.deposit(&escrow_id, &sender, &600, &token.address);
    escrow = client.get_escrow(&escrow_id).unwrap();
    assert_eq!(escrow.deposited_amount, 1000);
    assert_eq!(escrow.status, EscrowStatus::Funded);
}

#[test]
fn test_create_escrow_zero_amount() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);

    let result = client.try_create_escrow(&sender, &recipient, &0, &asset, &2000, &String::from_str(&env, ""));
    assert_eq!(result, Err(Ok(Error::InvalidAmount)));
}

#[test]
fn test_create_escrow_same_sender_recipient() {
    let env = Env::default();
    let (client, _admin, sender, _recipient, _token, asset) = setup_test(&env);

    let result = client.try_create_escrow(&sender, &sender, &1000, &asset, &2000, &String::from_str(&env, ""));
    assert_eq!(result, Err(Ok(Error::SameSenderRecipient)));
}

#[test]
fn test_create_escrow_unsupported_asset() {
    let env = Env::default();
    let (client, _admin, sender, recipient, _token, _asset) = setup_test(&env);

    let unsupported_asset = Asset {
        code: String::from_str(&env, "BAD"),
        issuer: Address::generate(&env),
    };

    let result = client.try_create_escrow(&sender, &recipient, &1000, &unsupported_asset, &2000, &String::from_str(&env, ""));
    assert_eq!(result, Err(Ok(Error::InvalidAsset)));
}

#[test]
fn test_deposit_wrong_sender() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    let wrong_sender = Address::generate(&env);
    token_admin.mint(&wrong_sender, &amount);

    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));

    let result = client.try_deposit(&escrow_id, &wrong_sender, &amount, &token.address);
    assert_eq!(result, Err(Ok(Error::WrongSender)));
}

#[test]
fn test_deposit_overflow() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &2000);

    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));

    let result = client.try_deposit(&escrow_id, &sender, &1500, &token.address);
    assert_eq!(result, Err(Ok(Error::InsufficientAmount)));
}

macro_rules! test_create_escrow_parametrized {
    ($($name:ident: $amount:expr,)*) => {
        $(
            #[test]
            fn $name() {
                let env = Env::default();
                let (client, _admin, sender, recipient, _token, asset) = setup_test(&env);
                let escrow_id = client.create_escrow(&sender, &recipient, &$amount, &asset, &2000, &String::from_str(&env, ""));
                let escrow = client.get_escrow(&escrow_id).unwrap();
                assert_eq!(escrow.amount, $amount);
            }
        )*
    }
}

test_create_escrow_parametrized! {
    test_create_100: 100,
    test_create_500: 500,
    test_create_1000: 1000,
}

#[test]
fn test_events_emitted() {
    let env = Env::default();
    let (client, _admin, sender, recipient, (token, token_admin), asset) = setup_test(&env);

    let amount = 1000;
    token_admin.mint(&sender, &amount);

    let escrow_id = client.create_escrow(&sender, &recipient, &amount, &asset, &2000, &String::from_str(&env, ""));
    
    // Check 'created' event
    let events = env.events().all();
    let created_event = events.into_iter().find(|e| {
        let topics = &e.1;
        topics.len() > 0 && Symbol::from_val(&env, &topics.get(0).unwrap()) == Symbol::new(&env, "created")
    }).unwrap();
    
    assert_eq!(created_event.0, client.address);
    let topics = created_event.1;
    assert_eq!(Symbol::from_val(&env, &topics.get(0).unwrap()), Symbol::new(&env, "created"));
    assert_eq!(u64::from_val(&env, &topics.get(1).unwrap()), escrow_id);
    assert_eq!(Address::from_val(&env, &created_event.2), sender);

    client.deposit(&escrow_id, &sender, &amount, &token.address);

    // Check 'deposit' event
    let events = env.events().all();
    let deposit_event = events.into_iter().find(|e| {
        let topics = &e.1;
        topics.len() > 0 && Symbol::from_val(&env, &topics.get(0).unwrap()) == Symbol::new(&env, "deposit")
    }).unwrap();
    
    let topics = deposit_event.1;
    assert_eq!(Symbol::from_val(&env, &topics.get(0).unwrap()), Symbol::new(&env, "deposit"));
    assert_eq!(u64::from_val(&env, &topics.get(1).unwrap()), escrow_id);
}
