# Contract deployment

`scripts/deploy_contracts.sh` builds, deploys, initialises and smoke-tests the
Soroban contracts. CI runs it automatically (#378).

## In CI (`.github/workflows/ci.yml`)

| Job | When | What |
| --- | --- | --- |
| `build-contracts-wasm` | every push and PR | Builds each deployable wasm, checks it against the network's contract size limit and uploads it as the `contracts-wasm` artifact. |
| `deploy-contracts-testnet` | push to `main`, or manual dispatch | Deploys to testnet, initialises each contract and runs the smoke tests. Uploads `testnet-contracts-<sha>` (`contracts.json`, `contracts.env` and the exact wasm deployed), kept 90 days. |

- **Redeploy by hand.** Use *Actions → CI → Run workflow*. The
  `deploy_contracts` input picks which contracts to deploy; leave it empty to
  skip deploying.
- **Concurrency.** Deployments never run in parallel, and a running one is
  never cancelled.
- **`TESTNET_DEPLOYER_SECRET`** (secret): a testnet secret key that becomes
  the admin of every deployed contract. Without it, each run generates a
  fresh friendbot-funded identity. Its key is lost when the runner ends, so
  set the secret if you need to administer the deployed contracts later.
- **`DEPLOY_CONTRACTS`** (repository variable): the default contract list
  for automatic deploys. It defaults to `escrow hub`.

## Locally

```bash
# Build and size-check only; nothing is deployed:
DRY_RUN=1 ./scripts/deploy_contracts.sh

# Deploy to testnet with a throwaway identity:
./scripts/deploy_contracts.sh

# Deploy with your own key, read from the environment and never printed:
STELLAR_DEPLOYER_SECRET=S... ./scripts/deploy_contracts.sh
```

The script needs the Stellar CLI (CI pins `30.0.1`), Rust with the
`wasm32v1-none` target, and `python3`. Outputs go to `deployments/<network>/`,
which is gitignored. See the header of the script for every option,
including `ALLOW_MAINNET`.

## What is deployed

The crate builds one wasm per contract, selected by a Cargo feature. One wasm
cannot hold both the escrow and the hub, because both export
`set_rate_limit_config` and five other entry points with the same names.
Native builds and tests always include every contract.

| Contract | Feature | Initialised with | Smoke test |
| --- | --- | --- | --- |
| `escrow` (PaymentEscrowContract) | `contract-escrow` | `init_escrow(admin)` | `get_platform_fee` returns a number; `get_total_escrow_volume` is 0 |
| `hub` (RemittanceHubContract) | `contract-hub` | `init_hub(admin, primary_oracle, secondary_oracle, max_staleness)` | `is_paused` is false; `version` is a number; `get_oracle_config` names the primary oracle |
| mock oracles | `contract-mocks` | `init_oracle(admin)` | Deployed only on test networks, as the hub's two oracles, when `PRIMARY_ORACLE_ID` is not set. |

Wasm is built with the size-optimised `release-wasm` profile and
`stellar contract build --optimize`. Current sizes:

- escrow: about 105 KB
- hub: about 64 KB
- mocks: about 26 KB

The testnet limit is 131,072 bytes.
