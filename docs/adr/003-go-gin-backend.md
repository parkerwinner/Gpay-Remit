# ADR-003: Use Go with Gin Framework for Backend API

## Status

Accepted

## Context

The backend needs to serve a REST API for user management, payment orchestration, KYC tracking, and compliance checks. We need a performant, maintainable technology choice.

## Decision

We will use Go 1.21+ with the Gin web framework for the backend API layer.

Key architectural choices:
- Gin for HTTP routing and middleware
- GORM for PostgreSQL database operations
- JWT-based authentication with refresh tokens
- Middleware chain: CORS, rate limiting, audit trail, error handling, version negotiation

## Consequences

### Positive

- Go's concurrency model handles high throughput with goroutines
- Gin provides fast HTTP routing with minimal overhead
- Strong standard library reduces dependency count
- Static typing catches bugs at compile time
- Excellent performance for I/O-bound API workloads
- Built-in support for graceful shutdown

### Negative

- Go lacks native REPL for rapid prototyping
- Generics support is still maturing
- Error handling verbosity compared to exception-based languages

### Risks

- Team may need Go experience ramp-up time
