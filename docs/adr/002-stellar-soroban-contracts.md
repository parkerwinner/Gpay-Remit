# ADR-002: Use Stellar Soroban for Smart Contracts

## Status

Accepted

## Context

Gpay-Remit needs a trustless escrow mechanism for cross-border payments. We need to decide on the blockchain platform and smart contract language for implementing the payment logic.

Key requirements:
- Low transaction fees for micro-payments
- Fast finality for real-time remittance
- Multi-currency support
- Escrow capabilities for conditional releases

## Decision

We will use Stellar Soroban (Rust-based smart contracts) for the on-chain payment escrow and remittance hub contracts.

- `PaymentEscrowContract`: Handles secure escrow deposits and conditional releases
- `RemittanceHubContract`: Manages remittance transfers, currency conversions, and invoice generation

## Consequences

### Positive

- Stellar's low transaction fees ($0.00001 per operation) enable cost-effective remittances
- ~5 second transaction finality provides near-instant settlement
- Built-in DEX for native multi-currency conversion
- Soroban's Rust SDK provides type safety and compile-time guarantees
- Persistent storage model supports complex stateful contracts
- Native token transfer capabilities via SEP-41

### Negative

- Soroban is a newer platform with a smaller ecosystem compared to EVM
- Rust learning curve for developers unfamiliar with the language
- Limited tooling compared to more mature blockchain platforms

### Risks

- Soroban ecosystem maturity may require custom tooling
- Contract upgrade patterns need careful design (addressed via upgradeable module)
