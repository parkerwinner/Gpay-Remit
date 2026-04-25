# Errors

This project uses Soroban `#[contracterror]` enums for explicit, stable error codes.

## Contract error enums

- `contracts/src/payment_escrow.rs`: `Error`
- `contracts/src/remittance_hub.rs`: `RemittanceError`
- `contracts/src/aml.rs`: `AmlError`
- `contracts/src/kyc.rs`: `KycError`
- `contracts/src/oracle.rs`: `OracleError`

## Documentation rule

Each error variant has an inline Rust doc comment explaining:

- When it can occur
- What the caller can do to resolve it (when applicable)

This is intended to help both frontend integrations and off-chain indexers map error codes to actionable user messages.

