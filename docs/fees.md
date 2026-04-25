# Fee Calculation (Backend)

The backend calculates a fee breakdown during remittance creation and exposes a
preview endpoint:

- `GET /api/v1/fees/calculate?amount=<number>`

## Breakdown

The response includes:

- `platform_fee`
- `forex_fee`
- `compliance_fee`
- `network_fee`
- `total_fee`

## Configuration

Fee configuration is provided via environment variables (basis points):

- `PLATFORM_FEE_BPS`
- `FOREX_FEE_BPS`
- `COMPLIANCE_FEE_BPS`
- `NETWORK_FEE_BPS`
- `MIN_FEE` (optional)
- `MAX_FEE` (optional)

These values are intended to mirror the on-chain fee structure configured in
the PaymentEscrow contract.
