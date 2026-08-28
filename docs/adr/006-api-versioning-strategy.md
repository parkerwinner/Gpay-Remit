# ADR-006: API Versioning Strategy

## Status

Accepted

## Context

As the API evolves, we need a strategy to introduce breaking changes without disrupting existing clients. We currently support v1 (deprecated) and v2 (current) API versions.

## Decision

We will use a hybrid versioning approach:

1. **Path-based versioning**: `/api/v1/...` and `/api/v2/...` for major version differences
2. **Header-based versioning**: `Accept-Version` header for fine-grained version selection
3. **Backward compatibility**: v1 remains functional but deprecated during transition periods

The `VersionMiddleware` extracts the requested version from either the path prefix or the `Accept-Version` header.

## Consequences

### Positive

- Clients can migrate at their own pace
- Path-based versioning is explicit and easy to understand
- Header-based versioning allows version selection without URL changes
- Deprecation warnings guide clients to upgrade

### Negative

- Maintaining multiple API versions increases code duplication
- Middleware complexity for version routing
- Documentation must cover all supported versions

### Risks

- Long-lived v1 support increases maintenance burden (mitigated by deprecation timeline)
