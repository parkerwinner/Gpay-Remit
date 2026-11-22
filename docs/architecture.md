# Gpay-Remit Architecture

## Overview

Gpay-Remit is a modern, high-performance platform for cross-border payments and instant remittances built on the Stellar network and Soroban smart contracts.

The platform combines an off-chain API engine for fast compliance, user management, and transaction orchestration with trustless on-chain Soroban escrow contracts and Stellar DEX settlement.

---

## 1. System Architecture

```mermaid
flowchart TD
    subgraph Client ["Client Layer"]
        ReactApp["React Web Application"]
        Wallet["Stellar Wallet (Freighter / Albedo / Ledger)"]
    end

    subgraph API ["Backend API Service (Go / Gin)"]
        Router["Gin HTTP Router & Middleware"]
        AuthM["Auth & 2FA Middleware"]
        RateM["Rate Limiter & Audit Middleware"]
        Handlers["API Handlers (Remittances, Invoices, Webhooks, KYC)"]
        Services["Domain Services (Escrow, Rates, Delivery)"]
        Workers["Background Workers (Retry, Materialized Views)"]
    end

    subgraph Storage ["Persistence & Cache Layer"]
        PG[("PostgreSQL Database (GORM)")]
        Redis[("Redis Cache & Rate Limit State")]
    end

    subgraph Stellar ["Stellar Blockchain & Soroban Contracts"]
        Horizon["Stellar Horizon RPC"]
        EscrowContract["PaymentEscrow Contract (Soroban)"]
        HubContract["RemittanceHub Contract (Soroban)"]
        StellarDEX["Stellar Decentralized Exchange (DEX)"]
    end

    subgraph External ["External Integrations"]
        Webhooks["Webhook Consumers"]
        Oracles["Exchange Rate Oracles"]
        KYC["KYC / AML Verifiers"]
    end

    ReactApp --> Router
    ReactApp -.-> Wallet
    Wallet -.-> Stellar
    Router --> AuthM --> RateM --> Handlers --> Services
    Services --> PG
    Services --> Redis
    Services --> Horizon
    Horizon --> EscrowContract
    Horizon --> HubContract
    HubContract --> StellarDEX
    Workers --> Services
    Workers --> Webhooks
    Services --> Oracles
    Services --> KYC
```

---

## 2. Core Architecture Layers

### 2.1 Smart Contract Layer (Soroban)
- **Technology**: Rust with Soroban SDK.
- **Contracts**:
  - `PaymentEscrowContract`: Enforces conditional escrow deposits, multi-party approvals, timelocks, and atomic releases/refunds.
  - `RemittanceHubContract`: Handles cross-border routing, fee distribution, and currency exchange via Stellar DEX payment paths.
- **Key Capabilities**:
  - Trustless escrow holds without custodial intermediary risk.
  - Multi-asset settlements (USDC, XLM, EURC, etc.).
  - Deterministic state machine with ledger-anchored event logs.

### 2.2 Backend API Layer (Go)
- **Technology**: Go 1.25+ with Gin web framework.
- **Key Modules**:
  - `handlers/`: RESTful JSON endpoints for authentication, remittances, invoices, webhooks, and analytics.
  - `middleware/`: Structured logging, TLS termination, CORS, CSRF, JWT validation, rate limiting, and security headers.
  - `services/`: Business logic for transaction orchestration, Stellar Horizon client interactions, and webhook delivery.
  - `workers/`: Background workers for scheduled retries, cache refreshes, and dead-letter queue processing.

### 2.3 Frontend Layer (React)
- **Technology**: React 18, React Router, Stellar SDK, and Freighter API.
- **Key Features**:
  - Interactive remittance submission and status tracking.
  - Invoice viewing, generation, and payments.
  - Multi-wallet connection (Freighter, Albedo, Ledger).
  - TypeScript contract definitions synchronized with the backend OpenAPI schema.

### 2.4 Storage & Cache Layer
- **PostgreSQL**: Stores relational user models, invoices, audit trails, and webhook logs with GORM migrations.
- **Redis**: Fast key-value store for rate limiting tokens, exchange rate caching, and distributed worker locks.

---

## 3. Data Flow Architecture

The diagram below details the end-to-end data flow when a user initiates a cross-border remittance:

```mermaid
sequenceDiagram
    autonumber
    actor Sender as Sender (Client)
    participant Frontend as React Frontend
    participant API as Go Backend API
    participant DB as PostgreSQL DB
    participant Stellar as Stellar Horizon / Soroban
    participant Receiver as Receiver / Webhook Consumer

    Sender->>Frontend: Fill remittance details & submit
    Frontend->>API: POST /api/v1/remittances (Bearer JWT)
    API->>DB: Verify user identity & KYC tier limits
    API->>Stellar: Create escrow on Soroban contract
    Stellar-->>API: Confirm escrow creation & lock funds
    API->>DB: Insert remittance record (status: pending)
    API-->>Frontend: 201 Created (remittance metadata & escrow ID)
    
    rect rgb(240, 248, 255)
        Note over API,Stellar: On-chain Settlement & Release
        API->>Stellar: Trigger conditional release / DEX path payment
        Stellar-->>API: On-chain transaction confirmed (TxHash)
    end

    API->>DB: Update remittance record (status: completed, tx_hash)
    API->>Receiver: Deliver webhook event payment.completed (HMAC-SHA256 signed)
    Receiver-->>API: 200 OK
    Frontend->>API: GET /api/v1/remittances/{id}
    API-->>Frontend: Status: completed with TxHash
```

---

## 4. Deployment Topology

The production and local deployment topologies leverage containerized microservices managed via Docker Compose or Kubernetes:

```mermaid
flowchart LR
    subgraph IngressLayer ["Ingress & Gateway"]
        Internet((Clients / Users)) --> LoadBalancer["TLS Load Balancer"]
        LoadBalancer --> NginxProxy["Nginx Reverse Proxy (:80 / :443)"]
    end

    subgraph ServiceLayer ["Application Containers"]
        NginxProxy --> FrontendApp["Frontend Web Container (:3000)"]
        NginxProxy --> BackendApp["Backend API Container (:8080)"]
    end

    subgraph DataLayer ["Data & Cache Containers"]
        BackendApp --> PostgresDB[("PostgreSQL DB (:5432)")]
        BackendApp --> RedisCache[("Redis (:6379)")]
    end

    subgraph Observability ["Telemetry & Monitoring"]
        BackendApp -.-> Prometheus["Prometheus Metrics (:9090)"]
        Prometheus -.-> Grafana["Grafana Visualizations (:3001)"]
    end

    subgraph BlockchainNetwork ["Blockchain Network"]
        BackendApp --> StellarHorizon["Stellar Horizon Node"]
        StellarHorizon --> SorobanRPC["Soroban RPC Network"]
    end
```

---

## 5. Security Architecture

1. **Authentication & Authorization**: High-entropy JWT tokens with short TTLs and refresh token rotation, backed by optional TOTP two-factor authentication (2FA).
2. **HMAC Webhook Signatures**: All outgoing webhook deliveries include an `X-Webhook-Signature` HMAC-SHA256 digest and `X-Webhook-Timestamp` header to guard against spoofing and replay attacks.
3. **Database Security**: Prepared statements via GORM to eliminate SQL injections; encrypted sensitive credentials using AES-GCM-256.
4. **Blockchain Integrity**: Soroban smart contracts enforce strict caller authorization (`require_auth`) and reentrancy protections.
