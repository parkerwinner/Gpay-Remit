# secrets

Resolves sensitive configuration values (JWT signing secrets today; any
future Stellar signing key or third-party API key the platform holds)
without the rest of the codebase calling `os.Getenv` directly for a secret.

## Providers

- **`env` (default)** — reads `os.Getenv(name)`. Identical to this repo's
  behavior before this package existed; no configuration change required
  for existing deployments.
- **`aws`** — reads from AWS Secrets Manager via the standard AWS SDK
  credential chain (environment variables, shared config/credentials
  files, or an EC2/ECS/Lambda instance role — no credentials are read or
  stored by this package directly). Values are cached in-process for 5
  minutes to avoid an API call on every config read.

Select with the `SECRETS_PROVIDER` environment variable (`env` or `aws`).

## Usage

```go
provider, err := secrets.NewProvider(ctx)
if err != nil {
    // handle
}

jwtSecret, err := provider.GetSecret(ctx, "JWT_SECRET")
```

For the `aws` provider, `name` is the AWS Secrets Manager secret's name or
ARN — create a secret with that exact name (e.g. `JWT_SECRET`) in Secrets
Manager, or change the name passed at the call site in `config/config.go`.

## Adding a new caller

Anywhere in the codebase reading a secret straight from `os.Getenv` should
migrate to `secrets.Provider.GetSecret` instead, so it automatically gains
AWS Secrets Manager support the same way `JWTSecret`/`JWTRefreshSecret` in
`config/config.go` did. Non-secret configuration (feature flags, URLs,
timeouts) should keep using `os.Getenv` directly — this package is for
values that would be a real security incident if leaked.
