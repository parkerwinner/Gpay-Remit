package secrets

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSecretsManagerClient struct {
	calls    int
	response *secretsmanager.GetSecretValueOutput
	err      error
	// lastSecretId records the SecretId passed on the most recent call, so
	// tests can assert the provider requested the right name.
	lastSecretID string
}

func (f *fakeSecretsManagerClient) GetSecretValue(
	_ context.Context,
	params *secretsmanager.GetSecretValueInput,
	_ ...func(*secretsmanager.Options),
) (*secretsmanager.GetSecretValueOutput, error) {
	f.calls++
	if params.SecretId != nil {
		f.lastSecretID = *params.SecretId
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func stringPtr(s string) *string { return &s }

func TestAWSSecretsManagerProvider_GetSecret_ReturnsValueFromClient(t *testing.T) {
	client := &fakeSecretsManagerClient{
		response: &secretsmanager.GetSecretValueOutput{
			SecretString: stringPtr("super-secret-jwt-value"),
		},
	}
	provider := newAWSSecretsManagerProviderWithClient(client, time.Minute)

	value, err := provider.GetSecret(context.Background(), "prod/jwt-secret")

	require.NoError(t, err)
	assert.Equal(t, "super-secret-jwt-value", value)
	assert.Equal(t, "prod/jwt-secret", client.lastSecretID)
}

func TestAWSSecretsManagerProvider_GetSecret_CachesWithinTTL(t *testing.T) {
	client := &fakeSecretsManagerClient{
		response: &secretsmanager.GetSecretValueOutput{
			SecretString: stringPtr("cached-value"),
		},
	}
	provider := newAWSSecretsManagerProviderWithClient(client, time.Minute)

	_, err := provider.GetSecret(context.Background(), "my-secret")
	require.NoError(t, err)
	_, err = provider.GetSecret(context.Background(), "my-secret")
	require.NoError(t, err)
	_, err = provider.GetSecret(context.Background(), "my-secret")
	require.NoError(t, err)

	assert.Equal(t, 1, client.calls, "expected only the first call to hit AWS; the rest should be served from cache")
}

func TestAWSSecretsManagerProvider_GetSecret_RefetchesAfterCacheExpires(t *testing.T) {
	client := &fakeSecretsManagerClient{
		response: &secretsmanager.GetSecretValueOutput{
			SecretString: stringPtr("value"),
		},
	}
	// Effectively-zero TTL: any two sequential calls should both hit AWS.
	provider := newAWSSecretsManagerProviderWithClient(client, 1*time.Nanosecond)

	_, err := provider.GetSecret(context.Background(), "my-secret")
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	_, err = provider.GetSecret(context.Background(), "my-secret")
	require.NoError(t, err)

	assert.Equal(t, 2, client.calls)
}

func TestAWSSecretsManagerProvider_GetSecret_CachesPerSecretNameIndependently(t *testing.T) {
	client := &fakeSecretsManagerClient{
		response: &secretsmanager.GetSecretValueOutput{
			SecretString: stringPtr("value"),
		},
	}
	provider := newAWSSecretsManagerProviderWithClient(client, time.Minute)

	_, err := provider.GetSecret(context.Background(), "secret-a")
	require.NoError(t, err)
	_, err = provider.GetSecret(context.Background(), "secret-b")
	require.NoError(t, err)

	assert.Equal(t, 2, client.calls, "different secret names must not share a cache entry")
}

func TestAWSSecretsManagerProvider_GetSecret_ReturnsErrorWhenClientFails(t *testing.T) {
	client := &fakeSecretsManagerClient{err: assert.AnError}
	provider := newAWSSecretsManagerProviderWithClient(client, time.Minute)

	_, err := provider.GetSecret(context.Background(), "my-secret")

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestAWSSecretsManagerProvider_GetSecret_ReturnsErrorForBinarySecret(t *testing.T) {
	client := &fakeSecretsManagerClient{
		response: &secretsmanager.GetSecretValueOutput{
			SecretString: nil,
			SecretBinary: []byte("binary-data"),
		},
	}
	provider := newAWSSecretsManagerProviderWithClient(client, time.Minute)

	_, err := provider.GetSecret(context.Background(), "my-secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "binary secrets are not supported")
}

func TestAWSSecretsManagerProvider_GetSecret_DoesNotCacheOnError(t *testing.T) {
	client := &fakeSecretsManagerClient{err: assert.AnError}
	provider := newAWSSecretsManagerProviderWithClient(client, time.Minute)

	_, err1 := provider.GetSecret(context.Background(), "my-secret")
	require.Error(t, err1)

	client.err = nil
	client.response = &secretsmanager.GetSecretValueOutput{SecretString: stringPtr("recovered")}

	value, err2 := provider.GetSecret(context.Background(), "my-secret")
	require.NoError(t, err2)
	assert.Equal(t, "recovered", value)
	assert.Equal(t, 2, client.calls, "a failed fetch must not be cached, so the next call retries")
}
