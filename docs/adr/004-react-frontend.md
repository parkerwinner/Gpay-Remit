# ADR-004: Use React.js for Frontend

## Status

Accepted

## Context

The frontend needs to provide a user-friendly interface for sending remittances, viewing invoices, and managing transactions. We need a component-based UI framework.

## Decision

We will use React.js with React Router for the frontend SPA.

Key choices:
- React 18 with functional components and hooks
- React Router v6 for client-side routing
- Stellar SDK for blockchain interactions
- CSS-based styling (no CSS-in-JS library)

## Consequences

### Positive

- Component-based architecture enables code reuse
- Large ecosystem of libraries and community support
- React Router provides clean URL-based navigation
- Stellar SDK integration for wallet and transaction handling
- Functional components with hooks simplify state management

### Negative

- Client-side routing requires server configuration for deep links
- Bundle size considerations for mobile users
- No built-in state management solution (addressed via Context API)

### Risks

- React version upgrades may require migration effort
