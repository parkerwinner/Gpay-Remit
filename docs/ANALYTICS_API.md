# Analytics API Documentation

## Overview

The Analytics API provides transaction statistics and metrics for administrators to monitor platform performance and business intelligence.

## Authentication

All analytics endpoints require admin-level authentication. Include your JWT token in the Authorization header:

```
Authorization: Bearer YOUR_JWT_TOKEN
```

## Endpoints

### 1. Volume Metrics

Get transaction volume statistics for a specified time period.

**Endpoint:** `GET /api/v1/analytics/volume`

**Query Parameters:**
- `period` (optional): Time period for metrics. Options: `daily`, `weekly`, `monthly`, `yearly`. Default: `daily`
- `start_date` (optional): Custom start date in `YYYY-MM-DD` format
- `end_date` (optional): Custom end date in `YYYY-MM-DD` format

**Example Request:**
```bash
curl -H "Authorization: Bearer TOKEN" \
  "https://api.gpay-remit.com/api/v1/analytics/volume?period=monthly"
```

**Example Response:**
```json
{
  "period": "monthly",
  "total_volume": 125000.50,
  "total_count": 450,
  "currency": "USD",
  "start_date": "2026-06-01",
  "end_date": "2026-07-01"
}
```

### 2. Fee Metrics

Get detailed fee collection statistics.

**Endpoint:** `GET /api/v1/analytics/fees`

**Query Parameters:**
- `period` (optional): Time period for metrics. Options: `daily`, `weekly`, `monthly`, `yearly`. Default: `daily`
- `start_date` (optional): Custom start date in `YYYY-MM-DD` format
- `end_date` (optional): Custom end date in `YYYY-MM-DD` format

**Example Request:**
```bash
curl -H "Authorization: Bearer TOKEN" \
  "https://api.gpay-remit.com/api/v1/analytics/fees?period=weekly"
```

**Example Response:**
```json
{
  "period": "weekly",
  "total_fees": 1250.75,
  "platform_fees": 625.00,
  "forex_fees": 312.50,
  "compliance_fees": 125.00,
  "network_fees": 188.25,
  "transaction_count": 125,
  "start_date": "2026-06-23",
  "end_date": "2026-06-30"
}
```

### 3. Success Rate Metrics

Get transaction success and failure statistics.

**Endpoint:** `GET /api/v1/analytics/success-rate`

**Query Parameters:**
- `period` (optional): Time period for metrics. Options: `daily`, `weekly`, `monthly`, `yearly`. Default: `daily`
- `start_date` (optional): Custom start date in `YYYY-MM-DD` format
- `end_date` (optional): Custom end date in `YYYY-MM-DD` format

**Example Request:**
```bash
curl -H "Authorization: Bearer TOKEN" \
  "https://api.gpay-remit.com/api/v1/analytics/success-rate?period=daily"
```

**Example Response:**
```json
{
  "period": "daily",
  "total_transactions": 100,
  "successful_transactions": 95,
  "failed_transactions": 3,
  "pending_transactions": 2,
  "success_rate": 95.0,
  "failure_rate": 3.0,
  "start_date": "2026-06-26",
  "end_date": "2026-06-27"
}
```

### 4. Top Corridors

Get the most popular currency corridors (source to destination pairs).

**Endpoint:** `GET /api/v1/analytics/top-corridors`

**Query Parameters:**
- `limit` (optional): Number of corridors to return (1-100). Default: `10`
- `period` (optional): Time period for metrics. Options: `daily`, `weekly`, `monthly`, `yearly`. Default: `monthly`
- `start_date` (optional): Custom start date in `YYYY-MM-DD` format
- `end_date` (optional): Custom end date in `YYYY-MM-DD` format

**Example Request:**
```bash
curl -H "Authorization: Bearer TOKEN" \
  "https://api.gpay-remit.com/api/v1/analytics/top-corridors?limit=5&period=monthly"
```

**Example Response:**
```json
{
  "corridors": [
    {
      "source_currency": "USD",
      "destination_currency": "EUR",
      "transaction_count": 250,
      "total_volume": 50000.00,
      "average_amount": 200.00,
      "total_fees": 500.00
    },
    {
      "source_currency": "USD",
      "destination_currency": "GBP",
      "transaction_count": 180,
      "total_volume": 36000.00,
      "average_amount": 200.00,
      "total_fees": 360.00
    }
  ],
  "limit": 5,
  "period": "monthly",
  "start_date": "2026-06-01",
  "end_date": "2026-07-01"
}
```

## Caching

Analytics endpoints implement intelligent caching to improve performance:

- **Daily metrics**: Cached for 5 minutes
- **Weekly metrics**: Cached for 15 minutes
- **Monthly metrics**: Cached for 30 minutes
- **Yearly metrics**: Cached for 1 hour

Custom date ranges are also cached based on the range duration.

## Error Responses

### 400 Bad Request
```json
{
  "error": "Invalid period. Valid values are: daily, weekly, monthly, yearly"
}
```

### 401 Unauthorized
```json
{
  "error": "Authorization header is required"
}
```

### 403 Forbidden
```json
{
  "error": "Forbidden: insufficient permissions"
}
```

### 500 Internal Server Error
```json
{
  "error": "Failed to retrieve volume metrics"
}
```

## Rate Limiting

Analytics endpoints are subject to the same rate limiting as other API endpoints. Admin users typically have higher rate limits.

## Best Practices

1. Use appropriate time periods for your use case
2. Leverage caching by avoiding frequent requests for the same data
3. Use custom date ranges for historical analysis
4. Monitor the response headers for cache information
5. Handle errors gracefully in your client application

## Examples

### Dashboard Volume Chart
```javascript
async function getVolumeData(period) {
  const response = await fetch(
    `https://api.gpay-remit.com/api/v1/analytics/volume?period=${period}`,
    {
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      }
    }
  );
  return await response.json();
}
```

### Custom Date Range Analysis
```bash
curl -H "Authorization: Bearer TOKEN" \
  "https://api.gpay-remit.com/api/v1/analytics/fees?start_date=2026-01-01&end_date=2026-06-30"
```

## Support

For questions or issues with the Analytics API, please refer to the main API documentation or contact the development team.
