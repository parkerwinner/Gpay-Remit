use criterion::{black_box, criterion_group, criterion_main, Criterion, BatchSize};
use soroban_sdk::{testutils::{Address as _, Ledger}, Address, Env, String};
use gpay_remit_contracts::payment_escrow::{PaymentEscrowContract, PaymentEscrowContractClient, Asset};

fn setup_env_with_token() -> (Env, PaymentEscrowContractClient<'static>, Address, Address) {
    let env = Env::default();
    env.mock_all_auths();
    
    let contract_id = env.register_contract(None, PaymentEscrowContract);
    let client = PaymentEscrowContractClient::new(&env, &contract_id);
    
    env.budget().reset_unlimited();
    
    let admin = Address::generate(&env);
    client.init_escrow(&admin);
    
    let token_admin = Address::generate(&env);
    let token_id = env.register_stellar_asset_contract(token_admin.clone());
    
    let asset = Asset {
        code: String::from_str(&env, "USDC"),
        issuer: admin.clone(),
    };
    client.add_supported_asset(&admin, &asset);
    
    (env, client, admin, token_id)
}

fn bench_create_escrow(c: &mut Criterion) {
    c.bench_function("create_escrow", |b| {
        b.iter_batched(|| {
            let (env, client, admin, _token) = setup_env_with_token();
            let sender = Address::generate(&env);
            let recipient = Address::generate(&env);
            let asset = Asset {
                code: String::from_str(&env, "USDC"),
                issuer: admin.clone(),
            };
            env.ledger().with_mut(|li| li.timestamp = 1000);
            (sender, recipient, asset, client)
        }, |(sender, recipient, asset, client)| {
            let _ = client.create_escrow(&sender, &recipient, black_box(&1000), black_box(&asset), black_box(&5000), black_box(&String::from_str(&client.env, "test")));
        }, BatchSize::SmallInput)
    });
}

fn bench_deposit(c: &mut Criterion) {
    c.bench_function("deposit_escrow", |b| {
        b.iter_batched(|| {
            let (env, client, admin, token_id) = setup_env_with_token();
            let sender = Address::generate(&env);
            let recipient = Address::generate(&env);
            let asset = Asset {
                code: String::from_str(&env, "USDC"),
                issuer: admin.clone(),
            };
            env.ledger().with_mut(|li| li.timestamp = 1000);
            
            let token_client = soroban_sdk::token::StellarAssetClient::new(&env, &token_id);
            token_client.mint(&sender, &5000);
            
            let escrow_id = client.create_escrow(&sender, &recipient, &1000, &asset, &5000, &String::from_str(&env, "test"));
            (escrow_id, sender, token_id, client)
        }, |(escrow_id, sender, token_id, client)| {
            let _ = client.deposit(black_box(&escrow_id), &sender, black_box(&1000), &token_id);
        }, BatchSize::SmallInput)
    });
}

fn bench_release(c: &mut Criterion) {
    c.bench_function("release_escrow", |b| {
        b.iter_batched(|| {
            let (env, client, admin, token_id) = setup_env_with_token();
            let sender = Address::generate(&env);
            let recipient = Address::generate(&env);
            let asset = Asset {
                code: String::from_str(&env, "USDC"),
                issuer: admin.clone(),
            };
            env.ledger().with_mut(|li| li.timestamp = 1000);
            
            let token_client = soroban_sdk::token::StellarAssetClient::new(&env, &token_id);
            token_client.mint(&sender, &5000);
            
            let escrow_id = client.create_escrow(&sender, &recipient, &1000, &asset, &5000, &String::from_str(&env, "test"));
            let _ = client.deposit(&escrow_id, &sender, &1000, &token_id);
            let _ = client.approve_escrow(&escrow_id, &admin);
            (escrow_id, recipient, token_id, client)
        }, |(escrow_id, recipient, token_id, client)| {
            let _ = client.release_escrow(black_box(&escrow_id), &recipient, &token_id);
        }, BatchSize::SmallInput)
    });
}

fn bench_kyc_checks(c: &mut Criterion) {
    c.bench_function("kyc_checks", |b| {
        b.iter_batched(|| {
            let (env, client, admin, _token) = setup_env_with_token();
            let oracle = Address::generate(&env);
            let _ = client.configure_kyc(&admin, &oracle, &true);
            let sender = Address::generate(&env);
            let recipient = Address::generate(&env);
            let asset = Asset {
                code: String::from_str(&env, "USDC"),
                issuer: admin.clone(),
            };
            (sender, recipient, asset, client)
        }, |(sender, recipient, asset, client)| {
            let _ = client.create_escrow(&sender, &recipient, black_box(&1000), black_box(&asset), black_box(&5000), black_box(&String::from_str(&client.env, "test")));
        }, BatchSize::SmallInput)
    });
}

criterion_group!(benches, bench_create_escrow, bench_deposit, bench_release, bench_kyc_checks);
criterion_main!(benches);
