// Package secrets provides an abstraction over where sensitive
// configuration values (JWT signing secrets, third-party API keys, and any
// future Stellar signing key the platform ends up holding) are read from,
// so the rest of the codebase never calls os.Getenv directly for a secret.
//
// Motivation (#193): several secrets in config.go were read straight from
// plain environment variables with no dedicated-secrets-manager option.
// Env vars are visible to anything with process/shell access on the host,
// get echoed into crash dumps and some logging/monitoring agents, and have
// no access audit trail, rotation support, or encryption at rest — a real
// secrets manager (AWS Secrets Manager here) addresses all of that. Env
// vars remain supported as the default/local-dev provider so this is not a
// breaking change for existing deployments; production deployments should
// set SECRETS_PROVIDER=aws.
package secrets

import (
	"context"
	"fmt"
	"os"
)

// Provider resolves a named secret to its current value.
type Provider interface {
	GetSecret(ctx context.Context, name string) (string, error)
}

// NewProvider builds the Provider selected by the SECRETS_PROVIDER
// environment variable ("env" [default] or "aws"). "aws" additionally
// requires AWS_REGION (or a default region configured via the standard AWS
// SDK credential chain) to be set.
func NewProvider(ctx context.Context) (Provider, error) {
	switch getEnvOrDefault("SECRETS_PROVIDER", "env") {
	case "aws":
		return NewAWSSecretsManagerProvider(ctx)
	case "env":
		return NewEnvProvider(), nil
	default:
		return nil, fmt.Errorf(
			"unsupported SECRETS_PROVIDER %q: must be \"env\" or \"aws\"",
			os.Getenv("SECRETS_PROVIDER"),
		)
	}
}

func getEnvOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
