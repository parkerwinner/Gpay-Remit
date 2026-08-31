# API Versioning Guide

## Overview

The Gpay-Remit API supports versioning to ensure backward compatibility when introducing breaking changes. This document explains how to use API versions and migrate between them.

## Supported Versions

- **v1**: Legacy version (Deprecated)
- **v2**: Current version (Recommended)

## Version Negotiation

You can specify the API version in three ways:

### 1. URL Path (Recommended)

```
GET /api/v1/remittances
GET /api/v2/remittances
```

### 2. X-API-Version Header

```
GET /api/remittances
X-API-Version: v2
```

### 3. Accept-Version Header

```
GET /api/remittances
Accept-Version: v2
```

## Default Behavior

If no version is specified, the API defaults to **v2** (the current version).

## Deprecation Warnings

When using deprecated API versions (v1), the response will include deprecation headers:

```
X-API-Deprecation-Warning: This API version is deprecated
X-API-Deprecation-Date: 2026-12-31
X-API-Sunset-Date: 2027-06-30
X-API-Deprecation-Info: Please migrate to v2. See documentation at /docs/migration
```

## Response Headers

All API responses include the version that was used:

```
X-API-Version: v2
```

## Error Response Format

All API errors return a standardized payload using the `ErrorResponse` structure:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "A human readable summary of the error",
    "details": {
      "field": "description"
    }
  }
}
```

- `code` is a stable error code such as `VALIDATION_ERROR`, `NOT_FOUND`, `UNAUTHORIZED`, or `INTERNAL_ERROR`.
- `message` is a client-friendly error summary.
- `details` is optional and may include validation metadata or extra information.

## Version-Specific Endpoints

Some endpoints may only be available in specific versions. If you request an endpoint that's not available in your requested version, you'll receive:

```json
{
  "error": "This endpoint is not available in the requested API version",
  "current_version": "v1",
  "required_versions": ["v2"]
}
```

## Migration from v1 to v2

### Breaking Changes

1. Enhanced analytics endpoints with better caching
2. Improved error response formats
3. Additional validation on request parameters

### Migration Steps

1. Update your client to specify `v2` in the URL or headers
2. Test all endpoints in a staging environment
3. Update error handling to accommodate new error formats
4. Deploy to production

### Example Migration

**Before (v1):**
```bash
curl -H "Authorization: Bearer TOKEN" \
  https://api.gpay-remit.com/api/v1/remittances
```

**After (v2):**
```bash
curl -H "Authorization: Bearer TOKEN" \
  https://api.gpay-remit.com/api/v2/remittances
```

## Timeline

- **Now**: v2 is stable and recommended
- **2026-12-31**: v1 officially deprecated
- **2027-06-30**: v1 sunset (no longer supported)

## Support

For questions or assistance with migration, please contact the development team or refer to the main API documentation.
