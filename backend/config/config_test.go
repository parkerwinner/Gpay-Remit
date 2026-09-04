package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

func TestGormLogLevel_LogsQueriesOutsideProduction(t *testing.T) {
	assert.Equal(t, gormlogger.Info, gormLogLevel("development"))
	assert.Equal(t, gormlogger.Info, gormLogLevel(""))
	assert.Equal(t, gormlogger.Info, gormLogLevel("staging"))
}

func TestGormLogLevel_OnlyLogsErrorsInProduction(t *testing.T) {
	assert.Equal(t, gormlogger.Error, gormLogLevel("production"))
	assert.Equal(t, gormlogger.Error, gormLogLevel("PRODUCTION"))
}

func TestNewGormLogger_ReturnsUsableLogger(t *testing.T) {
	assert.NotNil(t, newGormLogger("development"))
	assert.NotNil(t, newGormLogger("production"))
}

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

func validTestConfig() *Config {
	return &Config{
		Port:              "8080",
		Environment:       "development",
		DatabaseURL:       "postgres://user:pass@localhost:5432/db",
		StellarNetwork:    "testnet",
		HorizonURL:        "https://horizon-testnet.stellar.org",
		JWTSecret:         "a-valid-32-plus-character-secret-value",
		JWTRefreshSecret:  "another-valid-32-plus-character-secret",
		PlatformFeeBps:   50,
		ForexFeeBps:      25,
		ComplianceFeeBps: 10,
		NetworkFeeBps:    15,
		MinFee:           0,
		MaxFee:           100,
		DBMaxIdleConns:    10,
		DBMaxOpenConns:    100,
		DBConnMaxLifetime: 60 * 60 * 1000000000,
		EmailEnabled:      false,
		RedisDB:           0,
	}
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := validTestConfig()
	assert.NoError(t, cfg.Validate())
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := validTestConfig()
	cfg.Port = "not-a-port"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORT must be a valid port number")

	cfg.Port = "70000"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORT must be a valid port number")
}

func TestConfig_Validate_MissingDatabaseURL(t *testing.T) {
	cfg := validTestConfig()
	cfg.DatabaseURL = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL is required")

	cfg.DatabaseURL = "mysql://invalid"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL must be a valid postgres connection URI")
}

func TestConfig_Validate_InvalidEnvironment(t *testing.T) {
	cfg := validTestConfig()
	cfg.Environment = "invalid-env"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APP_ENV must be one of")
}

func TestConfig_Validate_InvalidStellarNetwork(t *testing.T) {
	cfg := validTestConfig()
	cfg.StellarNetwork = "invalid-net"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "STELLAR_NETWORK must be one of")
}

func TestConfig_Validate_InvalidHorizonURL(t *testing.T) {
	cfg := validTestConfig()
	cfg.HorizonURL = "ftp://invalid-url"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HORIZON_URL must be a valid HTTP/HTTPS URL")
}

func TestConfig_Validate_NegativeAndExcessiveFees(t *testing.T) {
	cfg := validTestConfig()
	cfg.PlatformFeeBps = -1
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PLATFORM_FEE_BPS must be non-negative")

	cfg = validTestConfig()
	cfg.PlatformFeeBps = 6000
	cfg.ForexFeeBps = 5000
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "total fee basis points cannot exceed 10000")
}

func TestConfig_Validate_MinFeeExceedsMaxFee(t *testing.T) {
	cfg := validTestConfig()
	cfg.MinFee = 50
	cfg.MaxFee = 10
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot exceed MAX_FEE")
}

func TestConfig_Validate_DBPoolConstraints(t *testing.T) {
	cfg := validTestConfig()
	cfg.DBMaxOpenConns = 0
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_OPEN_CONNS must be greater than 0")

	cfg = validTestConfig()
	cfg.DBMaxIdleConns = 150
	cfg.DBMaxOpenConns = 100
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_IDLE_CONNS (150) cannot exceed DB_MAX_OPEN_CONNS (100)")
}

func TestConfig_Validate_EmailSettings(t *testing.T) {
	cfg := validTestConfig()
	cfg.EmailEnabled = true
	cfg.SMTPHost = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP_HOST is required when EMAIL_ENABLED=true")
}
