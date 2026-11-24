# Gpay-Remit

A cross-border payment and remittance hub built on the Stellar network, enabling instant, low-cost international money transfers with multi-currency support.

## Overview

Gpay-Remit leverages Stellar's payment rails to provide:

- **Instant Remittances**: Near-instant cross-border transfers
- **Multi-Currency Support**: Automatic currency conversions via Stellar DEX
- **Escrow Services**: Secure transfers with conditional releases
- **Invoice Generation**: Automated invoice creation and tracking
- **Compliance Tools**: KYC/AML integration hooks
- **Low Fees**: Minimal transaction costs using Stellar network

## Tech Stack

- **Smart Contracts**: Soroban (Rust) - Escrow and payment logic
- **Backend**: Go 1.21+ with Gin framework, Stellar Go SDK
- **Database**: PostgreSQL with GORM ORM
- **Frontend**: React.js with Stellar SDK
- **Deployment**: Docker & Docker Compose

## Project Structure

```
Gpay-Remit/
├── contracts/          # Soroban smart contracts (Rust)
├── backend/           # Go API server
├── frontend/          # React.js web interface
├── docs/              # Architecture documentation
└── scripts/           # Setup and deployment scripts
```

## Architecture

For complete system diagrams, data flow sequences, and deployment topology, see [docs/architecture.md](docs/architecture.md).

## Quick Start

### Prerequisites

- Rust 1.75+ with `wasm32-unknown-unknown` target
- Soroban CLI (`cargo install soroban-cli`)
- Go 1.21+
- Node.js 18+
- Docker & Docker Compose
- Stellar account on Testnet

### Setup

```bash
# Clone the repository
git clone https://github.com/yourusername/Gpay-Remit.git
cd Gpay-Remit

# Run setup script
chmod +x scripts/setup.sh
./scripts/setup.sh

# Start local development environment
docker-compose up -d

# Deploy contracts to Stellar Testnet (needs the stellar CLI and jq)
./scripts/deploy_contracts.sh
```

### Contract deployment

`scripts/deploy_contracts.sh` builds each contract as its own Wasm module, checks it against the network's contract size limit, then uploads, deploys and initializes it. It then smoke-tests every deployed contract with read-only calls. Contract IDs are written to `deployments/<network>.json`.

The script reads these environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `NETWORK` | `testnet` | Stellar CLI network name |
| `STELLAR_DEPLOYER_SECRET` | unset | Deployer secret key. On testnet, if this is unset, a throwaway friendbot-funded key is used. |
| `CONTRACTS` | `mock-oracle mock-kyc mock-aml escrow hub` | Which contracts to deploy |
| `SKIP_OVERSIZE` | `0` | Set to `1` to skip, instead of fail on, contracts over the size limit |

**In CI:**

- **Automatic:** the `Deploy Contracts to Testnet` job in `.github/workflows/ci.yml` runs this script on every merge to `main`, after all test jobs pass.
- **Manual:** you can redeploy from *Actions → CI → Run workflow*, optionally choosing which contracts to deploy.
- **Results:** the deployed addresses are uploaded as the `testnet-contracts-<sha>` artifact and listed in the job summary.
- **Deployer key:** set the `STELLAR_DEPLOYER_SECRET` secret on the `testnet` environment so the deployed contracts keep a stable admin.

### Running the Application

**Backend:**

```bash
cd backend
go run main.go
# Server runs on http://localhost:8080
```

**Frontend:**

```bash
cd frontend
npm start
# UI runs on http://localhost:3000
```

## Configuration

Copy `.env.example` to `.env` and configure:

- `STELLAR_NETWORK`: testnet or pubnet (network passphrase is derived automatically)
- `STELLAR_HORIZON_URL`: Stellar Horizon API endpoint (`HORIZON_URL` is accepted as a backwards-compatible alias)
- `DATABASE_URL`: PostgreSQL connection string
- `DATABASE_READ_REPLICA_URL`: optional PostgreSQL standby; analytics and history reads use it when healthy and current (see [read replica operations](docs/operations/read-replica.md))
- `CONTRACT_ID`: Deployed Soroban contract ID

The GraphQL endpoint enforces depth and field-count limits; see [GraphQL query limits](docs/operations/graphql-limits.md). After a frontend build, `npm run analyze:bundle` reports JavaScript asset sizes and enforces entry/total bundle budgets.

## Testing

**Smart Contracts:**

```bash
cd contracts
cargo test
```

**Backend:**

```bash
cd backend
go test ./...
```

**Frontend:**

```bash
cd frontend
npm test
```

## Deployment

See [docs/deployment.md](docs/deployment.md) for production deployment guide.

## Security

- Store secrets in environment variables
- Implement KYC/AML checks before large transfers
- Use multi-signature for high-value escrows
- Regular security audits recommended

## Contributing

Contributions welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Resources

- [Stellar Documentation](https://developers.stellar.org/)
- [Soroban Documentation](https://soroban.stellar.org/)
- [Stellar Go SDK](https://github.com/stellar/go)
