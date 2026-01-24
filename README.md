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

# Deploy contracts to Stellar Testnet
chmod +x scripts/deploy_contracts.sh
./scripts/deploy_contracts.sh
```

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

- `STELLAR_NETWORK`: testnet or futurenet
- `HORIZON_URL`: Stellar Horizon API endpoint
- `DATABASE_URL`: PostgreSQL connection string
- `CONTRACT_ID`: Deployed Soroban contract ID

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
