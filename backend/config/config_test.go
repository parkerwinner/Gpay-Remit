package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearJWTEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_REFRESH_SECRET", "")
	t.Setenv("SECRETS_PROVIDER", "")
	// godotenv.Load() in LoadConfig would otherwise pick up a real .env
	// file's values and mask what this test is asserting about missing/
	// invalid env vars.
	t.Setenv("DATABASE_URL", "postgres://test")
}

func TestLoadConfig_RequiresJWTSecret(t *testing.T) {
	clearJWTEnv(t)

	_, err := LoadConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_SECRET")
}

func TestLoadConfig_RejectsShortJWTSecret(t *testing.T) {
	clearJWTEnv(t)
	t.Setenv("JWT_SECRET", "too-short")
	t.Setenv("JWT_REFRESH_SECRET", "also-a-valid-32-plus-character-secret-value")

	_, err := LoadConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 32 characters")
}

func TestLoadConfig_RejectsShortJWTRefreshSecret(t *testing.T) {
	clearJWTEnv(t)
	t.Setenv("JWT_SECRET", "a-valid-32-plus-character-secret-value")
	t.Setenv("JWT_REFRESH_SECRET", "short")

	_, err := LoadConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_REFRESH_SECRET must be at least 32 characters")
}

func TestLoadConfig_SucceedsWithValidSecretsFromEnvProvider(t *testing.T) {
	clearJWTEnv(t)
	t.Setenv("JWT_SECRET", "a-valid-32-plus-character-secret-value")
	t.Setenv("JWT_REFRESH_SECRET", "another-valid-32-plus-character-secret")

	cfg, err := LoadConfig()

	require.NoError(t, err)
	assert.Equal(t, "a-valid-32-plus-character-secret-value", cfg.JWTSecret)
	assert.Equal(t, "another-valid-32-plus-character-secret", cfg.JWTRefreshSecret)
}

func TestLoadConfig_RejectsUnknownSecretsProvider(t *testing.T) {
	clearJWTEnv(t)
	t.Setenv("JWT_SECRET", "a-valid-32-plus-character-secret-value")
	t.Setenv("JWT_REFRESH_SECRET", "another-valid-32-plus-character-secret")
	t.Setenv("SECRETS_PROVIDER", "not-a-real-provider")

	_, err := LoadConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SECRETS_PROVIDER")
}
