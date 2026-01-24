#!/bin/bash

set -e

echo " Setting up Gpay-Remit..."

# Check prerequisites
echo "Checking prerequisites..."

if ! command -v cargo &> /dev/null; then
    echo " Rust/Cargo not found. Please install from https://rustup.rs/"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo " Go not found. Please install Go 1.21+"
    exit 1
fi

if ! command -v node &> /dev/null; then
    echo " Node.js not found. Please install Node.js 18+"
    exit 1
fi

if ! command -v docker &> /dev/null; then
    echo " Docker not found. Please install Docker"
    exit 1
fi

echo " All prerequisites found"

# Setup Rust contracts
echo " Setting up Soroban contracts..."
cd contracts
rustup target add wasm32-unknown-unknown
cargo build --target wasm32-unknown-unknown --release
cd ..

# Setup Go backend
echo " Setting up Go backend..."
cd backend
go mod download
go mod tidy
cd ..

# Setup React frontend
echo " Setting up React frontend..."
cd frontend
npm install
cd ..

# Create .env files
echo " Creating environment files..."

if [ ! -f backend/.env ]; then
    cat > backend/.env << EOF
PORT=8080
DATABASE_URL=postgres://gpay:gpay_password@localhost:5432/gpay_remit?sslmode=disable
STELLAR_NETWORK=testnet
HORIZON_URL=https://horizon-testnet.stellar.org
NETWORK_PASSPHRASE=Test SDF Network ; September 2015
CONTRACT_ID=
ESCROW_CONTRACT_ID=
EOF
    echo " Created backend/.env"
fi

if [ ! -f frontend/.env ]; then
    cp frontend/.env.example frontend/.env
    echo " Created frontend/.env"
fi

echo " Setup complete!"
echo ""
echo "Next steps:"
echo "1. Start services: docker-compose up -d"
echo "2. Deploy contracts: ./scripts/deploy_contracts.sh"
echo "3. Update CONTRACT_ID in backend/.env"
echo "4. Start backend: cd backend && go run main.go"
echo "5. Start frontend: cd frontend && npm start"
