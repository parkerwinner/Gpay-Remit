# Events

This project uses **Soroban events** to make contract state changes observable off-chain.

## Standard format

All contract events emitted by `PaymentEscrowContract` and `RemittanceHubContract` use a consistent:

- **Topics**: `(gpayremit, <component>, <action>, <id>)`
  - `<component>` is one of: `escrow`, `hub`
  - `<action>` is a short symbol describing the change (e.g. `created`, `deposit`, `inv_paid`)
  - `<id>` is the primary identifier for indexing (e.g. `escrow_id`, `invoice_id`), or `0` when not applicable
- **Data**: `GpayEvent` (see `contracts/src/events.rs`)
  - `timestamp`: ledger timestamp
  - `actor`: the address that initiated the change (or the contract address for automated actions)
  - `amount`: amount associated with the event (or `0` when not applicable)
  - `status`: a short symbol representing the post-action state (or `na` when not applicable)
  - `data`: typed `EventData` payload for the action

## Indexing guidance

Indexers should primarily filter by:

- `component` + `action` (topics 2–3)
- `id` (topic 4)

## Notes

- The event schema is intentionally stable across contracts to simplify indexers.
- Some legacy “notification-style” events are now emitted through the same standardized structure.

