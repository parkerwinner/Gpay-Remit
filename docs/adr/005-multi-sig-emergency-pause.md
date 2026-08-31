# ADR-005: Multi-Signature Emergency Pause Mechanism

## Status

Accepted

## Context

The smart contracts handle real money in escrow and remittance operations. A single-admin pause mechanism creates a single point of failure and a security risk. If the admin key is compromised, an attacker could pause or unpause the contract at will.

We need a more robust emergency pause mechanism that requires multiple authorized parties to agree before pausing or unpausing the contract.

## Decision

We implement a multi-signature emergency pause mechanism where:

1. A set of authorized pause signers is registered during initialization
2. Pausing requires a quorum (majority) of signers to approve
3. Unpausing also requires quorum approval
4. Each signer can only vote once per pause/unpause action
5. Votes expire after a configurable timeout period
6. The contract emits events for all pause-related actions for audit purposes

New contract functions:
- `init_pause_signers(admin, signers, quorum)` - Register signers and quorum
- `add_pause_signer(admin, signer)` - Add a new signer
- `remove_pause_signer(admin, signer)` - Remove a signer
- `vote_pause(signer)` - Vote to pause the contract
- `vote_unpause(signer)` - Vote to unpause the contract
- `get_pause_signers()` - List current signers and quorum
- `get_pause_votes()` - Current vote status

## Consequences

### Positive

- Eliminates single point of failure for emergency operations
- Multiple parties must collude to abuse pause functionality
- Configurable quorum allows flexibility for different security requirements
- Vote expiry prevents stale votes from being accumulated
- Full audit trail via contract events

### Negative

- Increased complexity in contract initialization
- Slightly higher gas costs for pause/unpause operations
- Requires coordination among signers during emergencies

### Risks

- Quorum may be unreachable if signers are unavailable (mitigated by configurable quorum and timeout)
- Signer key management requires operational discipline
