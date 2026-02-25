#![no_main]
use libfuzzer_sys::fuzz_target;
use contracts::remittance_hub::calculate_fees;

// Helper: clamp to avoid overflows in test
fn clamp_amount(val: u128) -> u128 {
    val.min(1_000_000_000_000_000)
}

fuzz_target!(|data: (u128, u32, u8, u8)| {
    // Inputs: amount, rate, asset_type, tier
    let (amount, rate, asset_type, tier) = data;
    let amount = clamp_amount(amount);
    let rate = rate % 100_000; // avoid absurd rates
    let asset_type = asset_type % 4; // assume 4 asset types
    let tier = tier % 5; // assume 5 tiers

    // Call fee calculation logic
    let fee = calculate_fees(amount, rate, asset_type, tier);

    // Invariants
    assert!(fee <= amount, "Fee exceeds amount");
    assert!(fee >= 0, "Fee negative");
    // Optionally: check for expected math properties
    // e.g., monotonicity, tier boundaries, etc.
});

// Seed corpus with known edge cases
// (cargo-fuzz will pick up from corpus/fees/ if present)
