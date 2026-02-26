use criterion::{black_box, criterion_group, criterion_main, Criterion, BatchSize};
use soroban_sdk::{testutils::{Address as _, Ledger}, Address, Env, String, Symbol, Vec, symbol_short};
use gpay_remit_contracts::remittance_hub::{RemittanceHubContract, RemittanceHubContractClient, Asset, EscrowRequest};

fn setup_env_with_token() -> (Env, RemittanceHubContractClient<'static>, Address, Address, Address) {
    let env = Env::default();
    env.mock_all_auths();
    
    // Register the contract
    let contract_id = env.register_contract(None, RemittanceHubContract);
    let client = RemittanceHubContractClient::new(&env, &contract_id);
    
    // Reset budget to unlimited for benchmarking
    env.budget().reset_unlimited();
    
    let admin = Address::generate(&env);
    let oracle = Address::generate(&env);
    
    // Initialize the hub
    client.init_hub(&admin, &oracle, &oracle, &3600);
    
    // Register a token
    let token_admin = Address::generate(&env);
    let token_id = env.register_stellar_asset_contract(token_admin.clone());
    
    (env, client, admin, oracle, token_id)
}

fn bench_send_remittance(c: &mut Criterion) {
    c.bench_function("send_remittance", |b| {
        b.iter_batched(|| {
            let (env, client, _admin, _oracle, _token) = setup_env_with_token();
            let from = Address::generate(&env);
            let to = Address::generate(&env);
            (from, to, client)
        }, |(from, to, client)| {
            client.send_remittance(&from, &to, black_box(&100), black_box(&symbol_short!("USD")));
        }, BatchSize::SmallInput)
    });
}

fn bench_batch_create_escrows(c: &mut Criterion) {
    for size in [1, 5, 10].iter() {
        c.bench_function(&format!("batch_create_escrows_size_{}", size), |b| {
            b.iter_batched(|| {
                let (env, client, _admin, _oracle, _token) = setup_env_with_token();
                let sender = Address::generate(&env);
                let recipient = Address::generate(&env);
                let issuer = Address::generate(&env);
                
                let asset = Asset {
                    code: String::from_str(&env, "USDC"),
                    issuer: issuer.clone(),
                };
                
                let mut requests = Vec::new(&env);
                for _ in 0..*size {
                    requests.push_back(EscrowRequest {
                        recipient: recipient.clone(),
                        amount: 100,
                        asset: asset.clone(),
                        expiration_timestamp: 10000,
                    });
                }
                env.ledger().with_mut(|li| li.timestamp = 5000);
                (sender, requests, client)
            }, |(sender, requests, client)| {
                client.batch_create_escrows(&sender, black_box(&requests));
            }, BatchSize::SmallInput)
        });
    }
}

fn bench_batch_deposit(c: &mut Criterion) {
    for size in [1, 5, 10].iter() {
        c.bench_function(&format!("batch_deposit_size_{}", size), |b| {
            b.iter_batched(|| {
                let (env, client, _admin, _oracle, token_id) = setup_env_with_token();
                let sender = Address::generate(&env);
                let recipient = Address::generate(&env);
                let issuer = Address::generate(&env);
                
                let asset = Asset {
                    code: String::from_str(&env, "USDC"),
                    issuer: issuer.clone(),
                };
                
                let mut requests = Vec::new(&env);
                for _ in 0..*size {
                    requests.push_back(EscrowRequest {
                        recipient: recipient.clone(),
                        amount: 100,
                        asset: asset.clone(),
                        expiration_timestamp: 10000,
                    });
                }
                env.ledger().with_mut(|li| li.timestamp = 5000);
                
                // Mint tokens to sender
                let token_client = soroban_sdk::token::StellarAssetClient::new(&env, &token_id);
                token_client.mint(&sender, &1000000);
                
                let ids = client.batch_create_escrows(&sender, &requests);
                (sender, ids, token_id, client)
            }, |(sender, escrow_ids, token_id, client)| {
                client.batch_deposit(&sender, black_box(&escrow_ids), black_box(&token_id));
            }, BatchSize::SmallInput)
        });
    }
}

fn bench_batch_release(c: &mut Criterion) {
    for size in [1, 5, 10].iter() {
        c.bench_function(&format!("batch_release_size_{}", size), |b| {
            b.iter_batched(|| {
                let (env, client, _admin, _oracle, token_id) = setup_env_with_token();
                let sender = Address::generate(&env);
                let recipient = Address::generate(&env);
                let issuer = Address::generate(&env);
                
                let asset = Asset {
                    code: String::from_str(&env, "USDC"),
                    issuer: issuer.clone(),
                };
                
                let mut requests = Vec::new(&env);
                for _ in 0..*size {
                    requests.push_back(EscrowRequest {
                        recipient: recipient.clone(),
                        amount: 100,
                        asset: asset.clone(),
                        expiration_timestamp: 10000,
                    });
                }
                env.ledger().with_mut(|li| li.timestamp = 5000);
                
                // Mint tokens to sender
                let token_client = soroban_sdk::token::StellarAssetClient::new(&env, &token_id);
                token_client.mint(&sender, &1000000);
                
                let ids = client.batch_create_escrows(&sender, &requests);
                client.batch_deposit(&sender, &ids, &token_id);
                (recipient, ids, token_id, client)
            }, |(recipient, escrow_ids, token_id, client)| {
                client.batch_release(&recipient, black_box(&escrow_ids), black_box(&token_id));
            }, BatchSize::SmallInput)
        });
    }
}

criterion_group!(benches, bench_send_remittance, bench_batch_create_escrows, bench_batch_deposit, bench_batch_release);
criterion_main!(benches);
