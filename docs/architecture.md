# Gpay-Remit Architecture

## Overview

Gpay-Remit is a three-tier application built on the Stellar blockchain network for cross-border payments and remittances.

## Architecture Layers

### 1. Smart Contract Layer (Soroban)

**Technology**: Rust with Soroban SDK

**Contracts**:

- `PaymentEscrowContract`: Handles secure escrow deposits and conditional releases
- `RemittanceHubContract`: Manages remittance transfers and currency conversions

**Key Features**:

- Trustless escrow mechanism
- Multi-currency support via Stellar DEX integration
- Atomic transactions for secure transfers
- Oracle integration for exchange rates (planned)

### 2. Backend API Layer

**Technology**: Go 1.21+ with Gin framework

**Components**:

- REST API server for client interactions
- Stellar SDK integration for blockchain operations
- PostgreSQL database for off-chain data
- GORM ORM for database operations

**Responsibilities**:

- User management and KYC tracking
- Payment orchestration
- Invoice generation
- Transaction history
- Compliance checks

### 3. Frontend Layer

**Technology**: React.js with Stellar SDK

**Features**:

- User-friendly remittance interface
- Real-time transaction status
- Invoice viewing and management
- Wallet integration (planned)

## Data Flow

1. User initiates remittance via frontend
2. Backend validates request and user KYC status
3. Backend calls Soroban contract to create escrow
4. Contract locks funds on Stellar network
5. Backend monitors transaction status
6. Upon confirmation, backend updates database
7. Invoice generated and returned to user

## Security Considerations

- Private keys stored in secure environment variables
- KYC verification before large transfers
- Rate limiting on API endpoints
- Input validation at all layers
- Audit logging for compliance

## Scalability

- Horizontal scaling of backend services
- Database read replicas for queries
- Caching layer for frequent data (planned)
- CDN for frontend assets

## Future Enhancements

- Multi-signature support for high-value transfers
- Integration with fiat on/off ramps (anchors)
- Mobile application
- Advanced analytics dashboard
- Automated compliance reporting
