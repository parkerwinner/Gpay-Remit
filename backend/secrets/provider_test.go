package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_DefaultsToEnvProvider(t *testing.T) {
	provider, err := NewProvider(context.Background())

	require.NoError(t, err)
	assert.IsType(t, &EnvProvider{}, provider)
}

func TestNewProvider_ExplicitEnvProvider(t *testing.T) {
	t.Setenv("SECRETS_PROVIDER", "env")

	provider, err := NewProvider(context.Background())

	require.NoError(t, err)
	assert.IsType(t, &EnvProvider{}, provider)
}

func TestNewProvider_RejectsUnknownProviderName(t *testing.T) {
	t.Setenv("SECRETS_PROVIDER", "vault-of-mystery")

	_, err := NewProvider(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SECRETS_PROVIDER")
}
