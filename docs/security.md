# Security Notes

## Reentrancy protection

`PaymentEscrowContract` uses a simple storage-based reentrancy guard (`DataKey::ReentrancyGuard`) around methods that perform external calls (e.g. token transfers).

Pattern:

- Read guard flag from instance storage
- Abort if already set
- Set guard `true`
- Perform state checks + external calls
- Reset guard `false` before returning

This reduces the risk of reentrant execution across external contract calls in Soroban.

