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

## Validation (#380)

The version is checked against a fixed list of supported versions,
`supportedVersions` in
[`backend/middleware/versioning.go`](../backend/middleware/versioning.go). That
table is the only place where a version is added, deprecated or given a sunset
date.

| Request | Result |
| --- | --- |
| `/api/v2/...`, or `X-API-Version: v2` / `2` / `V2` | Served as v2 |
| No version anywhere | Served as the default (v2) |
| A blank version header | Treated as absent |
| `/api/v3/...`, or `X-API-Version: v7` | `400` "Unsupported API version" |
| `X-API-Version: v2.0`, `latest`, … | `400` "Invalid API version format" |
| `/api/v1/...` with `X-API-Version: v2` | `400` "API version in header does not match the version in the URL path" |

Header precedence is `X-API-Version`, then `Accept-Version`. A header and the
path must agree, because a client that says both means one of them is wrong.

Every `400` response has the same body. The requested value is never echoed
back:

```json
{
  "error": "Unsupported API version",
  "supported_versions": ["v1", "v2"],
  "default_version": "v2"
}
```

Responses carry `Vary: X-API-Version, Accept-Version`, so caches keep versions
apart.

## Deprecation Headers

Responses served under a deprecated version (currently v1) include the
standard headers:

```
Deprecation: @1798675200                     # RFC 9745, the deprecation date as Unix seconds (2026-12-31)
Sunset: Wed, 30 Jun 2027 00:00:00 GMT        # RFC 8594, when the version stops working
Link: <https://github.com/parkerwinner/Gpay-Remit/blob/main/docs/API_VERSIONING.md>; rel="deprecation"; type="text/html"
```

The earlier `X-API-*` headers are still sent for existing clients:

```
X-API-Deprecation-Warning: This API version is deprecated
X-API-Deprecation-Date: 2026-12-31
X-API-Sunset-Date: 2027-06-30
X-API-Deprecation-Info: Please migrate to v2. See documentation at https://github.com/parkerwinner/Gpay-Remit/blob/main/docs/API_VERSIONING.md
```

## Versioning Policy

1. **When a new version is needed.** A major version (`vN`) is introduced
   only for a breaking change. Examples: removing or renaming a field or
   endpoint, changing a type, or tightening validation so that previously
   valid requests fail. Additive changes ship in the current version.
2. **How many versions are supported.** At most two at a time: the current
   version, and the previous one while it is deprecated.
3. **Deprecation.** When a new version ships, the previous one is marked
   deprecated in `supportedVersions` with a deprecation date and a sunset
   date. From then on, every response carries the headers above.
4. **Sunset.** The sunset date is at least six months after the deprecation
   date. After it, the version is removed from `supportedVersions`. Requests
   naming it then get `400 Unsupported API version`; they are not silently
   served by another version.
5. **What clients should do.** Clients should log or alert on a `Deprecation`
   header, and send an explicit version (path or header) rather than relying
   on the default. The default moves to each new version when that version
   ships.

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
