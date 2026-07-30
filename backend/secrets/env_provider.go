package secrets

import (
	"context"
	"fmt"
	"os"
)

// EnvProvider reads secrets from process environment variables — the
// existing behavior for every secret in this codebase today. Kept as the
// default provider so local development and any deployment not yet
// migrated to a real secrets manager keep working unchanged.
type EnvProvider struct{}

// NewEnvProvider constructs an EnvProvider.
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{}
}

// GetSecret returns os.Getenv(name), or an error if the variable is unset
// or empty (matching config.go's existing required-secret validation, so
// switching a caller from os.Getenv to this provider changes nothing about
// what counts as "missing" for a required secret).
func (p *EnvProvider) GetSecret(_ context.Context, name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("environment variable %s is required but not set", name)
	}
	return value, nil
}
