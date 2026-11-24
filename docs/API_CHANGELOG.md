# Gpay-Remit API Changelog

All notable changes to the Gpay-Remit REST and WebSocket APIs are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

For detailed version negotiation rules and deprecation schedules, see [API Versioning Guide](API_VERSIONING.md).

---

## [Unreleased]

### Added
- Prometheus metrics for database connection pool utilization and Redis cache hit/miss ratio.
- Pre-configured Grafana dashboard JSON in `monitoring/grafana/dashboards/` with automated provisioning.
- Dedicated operational runbooks in `docs/runbook/` for incident response.

---

## [2.0.0] - 2026-08-31

### Added
- **API Versioning Subsystem**: Native support for `/api/v2/` URL prefixes alongside `Accept-Version` and `X-API-Version` headers.
- **Two-Factor Authentication (2FA)**: Added endpoints `/auth/mfa/setup`, `/auth/mfa/verify`, `/auth/mfa/disable`, and `/auth/mfa/status`.
- **Password Reset Flow**: Added endpoints `/auth/forgot-password` and `/auth/reset-password`.
- **RFC 9745 & RFC 8594 Deprecation Headers**: Automated `Deprecation`, `Sunset`, and `Link` headers for deprecated endpoints.
- **Webhook Timestamping**: Added `X-Webhook-Timestamp` header and replay tolerance verification.

### Changed
- **Error Response Payload Format**: Standardized all error responses to the uniform schema `{"error": {"code": "...", "message": "...", "details": {...}}}`.
- **Analytics API**: Re-architected `/analytics/` endpoints to stream pre-computed materialized view rollups with improved cache headers.
- **Remittance List Response**: Added pagination metadata (`page`, `page_size`, `total_count`, `total_pages`).

### Deprecated
- **API v1**: Flagged `/api/v1/` routes as deprecated. Deprecation date: 2026-12-31; Sunset date: 2027-06-30.

### Fixed
- Fixed numeric precision truncation on monetary amount fields in webhook delivery payloads by encoding amounts as exact decimal strings.
- Fixed soft-delete cascade behavior in payment escrow records.

### Security
- Mandatory 32-character minimum entropy requirement for JWT signing secret keys.
- Timing-safe HMAC comparison for all incoming webhook signature verifications.

#### Migration Notes (v1 -> v2)
1. **URL Prefix**: Update API calls from `https://api.gpay-remit.com/api/v1/...` to `https://api.gpay-remit.com/api/v2/...` (or supply header `Accept-Version: v2`).
2. **Error Handling**: Replace legacy `payload.message` inspection with structured inspection on `payload.error.code` and `payload.error.details`.
3. **Pagination**: Consumers reading remittance lists should update response parsing to consume `{ "data": [...], "pagination": {...} }`.

---

## [1.2.0] - 2026-05-15

### Added
- **Multi-Asset Escrow**: Added support for non-native assets and trustlines in escrow contracts (USDC, EURC).
- **Payment Request Endpoints**: Added endpoints for `/payment-requests` creation, cancellation, and acceptance.
- **Webhook Dead-Letter Replay**: Added `POST /webhooks/deliveries/:id/retry` endpoint for manual replay of dead-lettered webhooks.

### Changed
- Increased maximum webhook delivery backoff retry intervals to 24 hours.

### Fixed
- Resolved edge case where Horizon transaction timeouts would leave remittance records in unverified states.

---

## [1.1.0] - 2026-03-20

### Added
- **Exchange Rate Oracle Caching**: Added Redis caching layer for currency conversion rates with configurable TTL.
- **Audit Logging**: Added middleware tracking IP address, user agent, and before/after request snapshots.
- **GDPR Export Endpoints**: Added `/users/export` and `/users/delete` for compliance.

### Security
- Enabled strict Content Security Policy (CSP) and HTTP security headers (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`).

---

## [1.0.0] - 2026-01-10

### Added
- Initial public release of Gpay-Remit REST API.
- Endpoints for user registration, authentication, remittance creation, invoice generation, and webhook event streaming.
- Soroban smart contract integration for trustless payment escrows.
