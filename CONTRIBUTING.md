# Contributing to Gpay-Remit

Thank you for contributing to Gpay-Remit! This guide outlines development practices and environment requirements.

## Prerequisites

- **Go**: Version `1.25.0` or higher is required for backend development. All CI workflows (`ci.yml`, `openapi-validation.yml`) enforce Go 1.25.
- **Node.js**: Version `18` or higher for frontend development.
- **Rust / Soroban CLI**: Stable toolchain for smart contract development.
- **Docker & Docker Compose**: For local infrastructure services (PostgreSQL, Redis).

## Development Setup

### Local Development with Docker Compose

To spin up all required dependencies (PostgreSQL 15, Redis 7, Backend, and Frontend) with development volume mounts and live reload:

```bash
make dev
# Or directly via docker compose:
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

### Backend

1. Navigate to the backend directory:
   ```bash
   cd backend
   ```
2. Verify Go version:
   ```bash
   go version # Must be 1.25.0+
   ```
3. Copy environment configuration and configure required variables (ensure `JWT_SECRET` is at least 32 characters):
   ```bash
   cp .env.example .env
   ```
4. Download dependencies and run tests:
   ```bash
   go mod download
   go test -race ./...
   ```

### CI Workflows

All pull requests run automated CI workflows that validate:
- Smart contract builds and tests
- Backend compilation, tests with race detection, and test coverage across Go 1.25
- OpenAPI schema validation against backend handlers
- Frontend linting and test suites
