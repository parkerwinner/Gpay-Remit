package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/secrets"
)

type Config struct {
	Port              string
	Environment       string
	DatabaseURL       string
	StellarNetwork    string
	HorizonURL        string
	ContractID        string
	EscrowContractID  string
	NetworkPassphrase string
	JWTSecret         string
	JWTRefreshSecret  string

	// Fee configuration (basis points, i.e. 100 bps = 1%)
	//
	// NOTE: These values are intended to mirror the fee structure configured in
	// the on-chain escrow contract (PaymentEscrow). Until the backend adds a
	// direct Soroban RPC read of the contract's fee config, these env-backed
	// values act as the source of truth for API calculations.
	PlatformFeeBps   int
	ForexFeeBps      int
	ComplianceFeeBps int
	NetworkFeeBps    int
	MinFee           float64
	MaxFee           float64

	// Database connection pool settings
	DBMaxIdleConns    int
	DBMaxOpenConns    int
	DBConnMaxLifetime time.Duration

	// Email configuration
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	EmailEnabled bool

	// Redis configuration
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Exchange rate oracle/pricing provider
	ExchangeRateAPIURL string

	// Sandbox mode — when true the backend uses testnet defaults,
	// reduced fee rates, and relaxed validation suitable for
	// development and integration testing. Controlled by the
	// SANDBOX_MODE environment variable.
	SandboxMode bool
}

func LoadConfig() (*Config, error) {
	godotenv.Load()

	// Secrets (#193): resolved through the secrets package rather than a
	// direct os.Getenv call, so JWT_SECRET/JWT_REFRESH_SECRET can be
	// migrated to a real secrets manager (AWS Secrets Manager, selected
	// via SECRETS_PROVIDER=aws) without touching this call site again.
	// Defaults to reading plain environment variables (SECRETS_PROVIDER
	// unset or "env"), so existing deployments are unaffected.
	ctx := context.Background()
	secretsProvider, err := secrets.NewProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("initializing secrets provider: %w", err)
	}

	jwtSecret, err := secretsProvider.GetSecret(ctx, "JWT_SECRET")
	if err != nil {
		return nil, err
	}
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters long")
	}

	jwtRefreshSecret, err := secretsProvider.GetSecret(ctx, "JWT_REFRESH_SECRET")
	if err != nil {
		return nil, err
	}
	if len(jwtRefreshSecret) < 32 {
		return nil, fmt.Errorf("JWT_REFRESH_SECRET must be at least 32 characters long")
	}

	sandbox := getEnvOrDefault("SANDBOX_MODE", "false") == "true"

	// When sandbox mode is active, apply testnet-friendly defaults that
	// differ from production. Individual env vars still take precedence
	// so developers can override any single value.
	stellarNetwork := getEnvOrDefault("STELLAR_NETWORK", "testnet")
	horizonURL := getEnvOrDefault("HORIZON_URL", "https://horizon-testnet.stellar.org")
	networkPassphrase := getEnvOrDefault("NETWORK_PASSPHRASE", "Test SDF Network ; September 2015")
	platformFeeBps := getEnvAsInt("PLATFORM_FEE_BPS", 50)
	forexFeeBps := getEnvAsInt("FOREX_FEE_BPS", 25)
	complianceFeeBps := getEnvAsInt("COMPLIANCE_FEE_BPS", 10)
	networkFeeBps := getEnvAsInt("NETWORK_FEE_BPS", 15)

	if sandbox {
		if !isEnvSet("STELLAR_NETWORK") {
			stellarNetwork = "testnet"
		}
		if !isEnvSet("HORIZON_URL") {
			horizonURL = "https://horizon-testnet.stellar.org"
		}
		if !isEnvSet("NETWORK_PASSPHRASE") {
			networkPassphrase = "Test SDF Network ; September 2015"
		}
		// Reduced fee rates for sandbox — lower barrier for testing
		if !isEnvSet("PLATFORM_FEE_BPS") {
			platformFeeBps = 10
		}
		if !isEnvSet("FOREX_FEE_BPS") {
			forexFeeBps = 5
		}
		if !isEnvSet("COMPLIANCE_FEE_BPS") {
			complianceFeeBps = 0
		}
		if !isEnvSet("NETWORK_FEE_BPS") {
			networkFeeBps = 0
		}
	}

	cfg := &Config{
		Port:              getEnvOrDefault("PORT", "8080"),
		Environment:       getEnvOrDefault("APP_ENV", "development"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		StellarNetwork:    stellarNetwork,
		HorizonURL:        horizonURL,
		ContractID:        os.Getenv("CONTRACT_ID"),
		EscrowContractID:  os.Getenv("ESCROW_CONTRACT_ID"),
		NetworkPassphrase: networkPassphrase,
		JWTSecret:         jwtSecret,
		JWTRefreshSecret:  jwtRefreshSecret,

		PlatformFeeBps:   platformFeeBps,
		ForexFeeBps:      forexFeeBps,
		ComplianceFeeBps: complianceFeeBps,
		NetworkFeeBps:    networkFeeBps,
		MinFee:           getEnvAsFloat("MIN_FEE", 0),
		MaxFee:           getEnvAsFloat("MAX_FEE", 0),

		DBMaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 10),
		DBMaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 100),
		DBConnMaxLifetime: time.Duration(getEnvAsInt("DB_CONN_MAX_LIFETIME_MIN", 60)) * time.Minute,

		SMTPHost:     getEnvOrDefault("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:     getEnvOrDefault("SMTP_PORT", "465"),
		SMTPUser:     os.Getenv("SMTP_USER"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     getEnvOrDefault("SMTP_FROM", os.Getenv("SMTP_USER")),
		EmailEnabled: getEnvOrDefault("EMAIL_ENABLED", "false") == "true",

		RedisAddr:     getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		ExchangeRateAPIURL: getEnvOrDefault("EXCHANGE_RATE_API_URL", "https://open.er-api.com/v6/latest"),

		SandboxMode: sandbox,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks all required configuration values and constraints.
func (c *Config) Validate() error {
	var errs []string

	// 1. Port
	if strings.TrimSpace(c.Port) == "" {
		errs = append(errs, "PORT is required")
	} else {
		var portNum int
		if _, err := fmt.Sscanf(c.Port, "%d", &portNum); err != nil || portNum < 1 || portNum > 65535 {
			errs = append(errs, fmt.Sprintf("PORT must be a valid port number between 1 and 65535 (got %q)", c.Port))
		}
	}

	// 2. DatabaseURL
	if strings.TrimSpace(c.DatabaseURL) == "" {
		errs = append(errs, "DATABASE_URL is required")
	} else if !strings.HasPrefix(c.DatabaseURL, "postgres://") && !strings.HasPrefix(c.DatabaseURL, "postgresql://") {
		errs = append(errs, "DATABASE_URL must be a valid postgres connection URI (starting with postgres:// or postgresql://)")
	}

	// 3. Environment
	validEnvs := map[string]bool{
		"development": true,
		"staging":     true,
		"production":  true,
		"test":        true,
	}
	if !validEnvs[strings.ToLower(c.Environment)] {
		errs = append(errs, fmt.Sprintf("APP_ENV must be one of 'development', 'staging', 'production', 'test' (got %q)", c.Environment))
	}

	// 4. JWT Secrets
	if len(c.JWTSecret) < 32 {
		errs = append(errs, "JWT_SECRET must be at least 32 characters long")
	}
	if len(c.JWTRefreshSecret) < 32 {
		errs = append(errs, "JWT_REFRESH_SECRET must be at least 32 characters long")
	}

	// 5. Stellar Network & Horizon URL
	validNetworks := map[string]bool{
		"testnet":    true,
		"public":     true,
		"standalone": true,
		"futurenet":  true,
	}
	if !validNetworks[strings.ToLower(c.StellarNetwork)] {
		errs = append(errs, fmt.Sprintf("STELLAR_NETWORK must be one of 'testnet', 'public', 'futurenet', 'standalone' (got %q)", c.StellarNetwork))
	}
	if !strings.HasPrefix(c.HorizonURL, "http://") && !strings.HasPrefix(c.HorizonURL, "https://") {
		errs = append(errs, fmt.Sprintf("HORIZON_URL must be a valid HTTP/HTTPS URL (got %q)", c.HorizonURL))
	}

	// 6. Fee validation
	if c.PlatformFeeBps < 0 {
		errs = append(errs, "PLATFORM_FEE_BPS must be non-negative")
	}
	if c.ForexFeeBps < 0 {
		errs = append(errs, "FOREX_FEE_BPS must be non-negative")
	}
	if c.ComplianceFeeBps < 0 {
		errs = append(errs, "COMPLIANCE_FEE_BPS must be non-negative")
	}
	if c.NetworkFeeBps < 0 {
		errs = append(errs, "NETWORK_FEE_BPS must be non-negative")
	}
	totalBps := c.PlatformFeeBps + c.ForexFeeBps + c.ComplianceFeeBps + c.NetworkFeeBps
	if totalBps > 10000 {
		errs = append(errs, fmt.Sprintf("total fee basis points cannot exceed 10000 (100%%), got %d bps", totalBps))
	}
	if c.MinFee < 0 {
		errs = append(errs, "MIN_FEE must be non-negative")
	}
	if c.MaxFee < 0 {
		errs = append(errs, "MAX_FEE must be non-negative")
	}
	if c.MaxFee > 0 && c.MinFee > c.MaxFee {
		errs = append(errs, fmt.Sprintf("MIN_FEE (%v) cannot exceed MAX_FEE (%v)", c.MinFee, c.MaxFee))
	}

	// 7. Database Connection Pool
	if c.DBMaxIdleConns < 0 {
		errs = append(errs, "DB_MAX_IDLE_CONNS must be non-negative")
	}
	if c.DBMaxOpenConns <= 0 {
		errs = append(errs, "DB_MAX_OPEN_CONNS must be greater than 0")
	}
	if c.DBMaxIdleConns > c.DBMaxOpenConns {
		errs = append(errs, fmt.Sprintf("DB_MAX_IDLE_CONNS (%d) cannot exceed DB_MAX_OPEN_CONNS (%d)", c.DBMaxIdleConns, c.DBMaxOpenConns))
	}
	if c.DBConnMaxLifetime <= 0 {
		errs = append(errs, "DB_CONN_MAX_LIFETIME must be positive")
	}

	// 8. Email Configuration (when enabled)
	if c.EmailEnabled {
		if strings.TrimSpace(c.SMTPHost) == "" {
			errs = append(errs, "SMTP_HOST is required when EMAIL_ENABLED=true")
		}
		if strings.TrimSpace(c.SMTPPort) == "" {
			errs = append(errs, "SMTP_PORT is required when EMAIL_ENABLED=true")
		}
		if strings.TrimSpace(c.SMTPUser) == "" {
			errs = append(errs, "SMTP_USER is required when EMAIL_ENABLED=true")
		}
		if strings.TrimSpace(c.SMTPFrom) == "" {
			errs = append(errs, "SMTP_FROM is required when EMAIL_ENABLED=true")
		}
	}

	// 9. Redis Configuration
	if c.RedisDB < 0 {
		errs = append(errs, "REDIS_DB must be non-negative")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed with %d error(s):\n  - %s", len(errs), strings.Join(errs, "\n  - "))
	}
	return nil
}

// gormLogLevel picks the GORM log verbosity for the given environment. Every
// non-production environment gets Info, which logs every executed query
// (including bound params) plus its execution time. Production only logs
// errors, to avoid leaking query data and flooding logs.
func gormLogLevel(env string) gormlogger.LogLevel {
	if strings.ToLower(env) == "production" {
		return gormlogger.Error
	}
	return gormlogger.Info
}

// newGormLogger builds the GORM logger for the given environment.
func newGormLogger(env string) gormlogger.Interface {
	return gormlogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormLogLevel(env),
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
}

func InitDB(cfg *Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: newGormLogger(cfg.Environment),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	// Start monitoring connection pool periodically
	go MonitorConnectionPool(sqlDB)

	return db, nil
}

// MonitorConnectionPool monitors database connection pool stats and logs alerts
func MonitorConnectionPool(db *sql.DB) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := db.Stats()

		// Log connection pool metrics
		logger.Log.WithFields(map[string]interface{}{
			"open_connections":  stats.OpenConnections,
			"in_use":             stats.InUse,
			"idle":               stats.Idle,
			"wait_count":         stats.WaitCount,
			"wait_duration_ms":   stats.WaitDuration.Milliseconds(),
			"max_idle_closed":    stats.MaxIdleClosed,
			"max_lifetime_closed": stats.MaxLifetimeClosed,
		}).Debug("Database connection pool stats")

		// Alert if connections are running low
		if stats.OpenConnections >= 80 {
			logger.Log.WithFields(map[string]interface{}{
				"open_connections": stats.OpenConnections,
				"in_use":           stats.InUse,
				"threshold":        "80%",
			}).Warn("Database connection pool nearing capacity")
		}

		// Alert if wait queue is building up
		if stats.WaitCount > 0 {
			logger.Log.WithFields(map[string]interface{}{
				"wait_count":     stats.WaitCount,
				"wait_duration":  stats.WaitDuration.Seconds(),
				"connections":   stats.OpenConnections,
			}).Warn("Queries waiting for database connections")
		}
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	var value int
	fmt.Sscanf(valueStr, "%d", &value)
	return value
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	var value float64
	fmt.Sscanf(valueStr, "%f", &value)
	return value
}

// isEnvSet returns true when the given environment variable is explicitly set
// (even to an empty string). This distinguishes "not set" from "set to ''".
func isEnvSet(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

// ResetSandbox reverts a Config instance to sandbox defaults in-place. This is
// useful for tests or runtime switches that need to restore sandbox-friendly
// values after a temporary override.
func (c *Config) ResetSandbox() {
	c.SandboxMode = true
	c.StellarNetwork = "testnet"
	c.HorizonURL = "https://horizon-testnet.stellar.org"
	c.NetworkPassphrase = "Test SDF Network ; September 2015"
	c.PlatformFeeBps = 10
	c.ForexFeeBps = 5
	c.ComplianceFeeBps = 0
	c.NetworkFeeBps = 0
	c.MinFee = 0
	c.MaxFee = 0
}
