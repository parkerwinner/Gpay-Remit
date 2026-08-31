package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvProvider_GetSecret_ReturnsSetVariable(t *testing.T) {
	t.Setenv("TEST_ENV_SECRET", "the-value")
	provider := NewEnvProvider()

	value, err := provider.GetSecret(context.Background(), "TEST_ENV_SECRET")

	require.NoError(t, err)
	assert.Equal(t, "the-value", value)
}

func TestEnvProvider_GetSecret_ErrorsWhenUnset(t *testing.T) {
	provider := NewEnvProvider()

	_, err := provider.GetSecret(context.Background(), "TEST_ENV_SECRET_DEFINITELY_UNSET")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "required but not set")
}

func TestEnvProvider_GetSecret_ErrorsWhenEmpty(t *testing.T) {
	t.Setenv("TEST_ENV_SECRET_EMPTY", "")
	provider := NewEnvProvider()

	_, err := provider.GetSecret(context.Background(), "TEST_ENV_SECRET_EMPTY")

	require.Error(t, err)
}
