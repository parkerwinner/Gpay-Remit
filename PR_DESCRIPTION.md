# Fix All CLI Issues and Add Missing Test Suites

## Overview

This PR fixes all existing CLI and compilation errors across the frontend, backend, and smart contract projects. It also adds crucial testing suites for fuzz testing, integration testing, and load testing as requested in issues #251, #252, #253, and #304.

## Issues Resolved

Closes #251: Missing Integration Tests for Webhooks
Closes #252: No Load Testing Suite
Closes #253: Missing Contract Fuzzing Tests
Closes #304: FIX ALL CLI ISSUES

## Summary of Changes

### 🎯 Issue #304: Fix All CLI Issues

**Branch:** `gpay-remit-cli-fixes`

- Fixed `cargo test` compilation issues in the `contracts` package.
- Pinned Rust dependencies (`ed25519-dalek`) in the `soroban-env-host` build graph to resolve `ChaCha20Rng` trait bound errors.
- Fixed `Recurring` and `RecurringHistory` duplicate definitions in `contracts/src/payment_escrow.rs`.
- Corrected the `InvoiceCreated` event signature to pass the missing `amount` argument.
- Removed unused imports and fixed unused variables (e.g., `_data` in test macros).
- Resolved a `symbol_short!` 10-character limit violation by renaming `signer_add` to `sig_add`.
- Fixed the frontend missing `lint` script by adding `"lint": "eslint src"` to `package.json`.
- Removed unused `useContext` import in `frontend/src/App.js` to silence the linter.

### 🎯 Issue #253: Missing Contract Fuzzing Tests

- Added `proptest` coverage for all primary smart contract methods.
- Created `contracts/tests/fuzz_remittance.rs` for `generate_invoice` and `send_remittance`.
- Created `contracts/tests/fuzz_escrow.rs` for `deposit` logic.
- Ensures all contract mathematical and state limits don't panic under heavy fuzzing loads.

### 🎯 Issue #251: Missing Integration Tests for Webhooks

- Added `backend/integration_test.go` webhook end-to-end flow.
- Used `testcontainers-go` to spawn a mock server.
- Sets webhook expectations and verifies that the backend successfully triggers the `POST` request upon completion.

### 🎯 Issue #252: No Load Testing Suite

- Added a robust `k6` load test in `backend/loadtest/k6_test.js`.
- Included stages to simulate real-world spikes (up to 50 target concurrent users over a minute).
- Configured thresholds for p(95) duration under 500ms to ensure the backend meets performance targets.

## Testing

All features include comprehensive test coverage:

- `cargo test` now compiles cleanly and passes all fuzz tests.
- `npm run lint` now returns exit code 0 on the frontend.
- Backend load testing and webhook flows can be run natively or via docker.

Run tests:

```bash
cd contracts
cargo test
```

```bash
cd frontend
npm run lint
```

```bash
cd backend
go test -tags=integration ./...
k6 run loadtest/k6_test.js
```

## Checklist

- [x] Code follows project style guidelines
- [x] Tests added and passing
- [x] No breaking changes
- [x] All CLI and compilation issues fixed
- [x] Webhook integration test using testcontainers
- [x] K6 load testing suite added
- [x] Proptest fuzzing for smart contracts added

**Ready for review!** 🚀
