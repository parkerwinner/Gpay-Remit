package config

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"github.com/stellar/go/network"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Gpay-Remit/gpay-remit/logger"
	"github.com/Gpay-Remit/gpay-remit/secrets"
)

var connectionPoolMonitor = struct {
	sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}{}

const (
	StellarNetworkTestnet = "testnet"
	StellarNetworkPubnet  = "pubnet"
)

type stellarNetworkSettings struct {
	horizonURL string
	passphrase string
	hosts      []string
}

var stellarNetworkSettingsByNetwork = map[string]stellarNetworkSettings{
	StellarNetworkTestnet: {
		horizonURL: "https://horizon-testnet.stellar.org",
		passphrase: network.TestNetworkPassphrase,
		hosts:      []string{"horizon-testnet.stellar.org"},
	},
	StellarNetworkPubnet: {
		horizonURL: "https://horizon.stellar.org",
		passphrase: network.PublicNetworkPassphrase,
		hosts:      []string{"horizon.stellar.org", "horizon-mainnet.stellar.org"},
	},
}

func canonicalStellarNetwork(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StellarNetworkTestnet:
		return StellarNetworkTestnet, true
	case StellarNetworkPubnet, "public", "mainnet":
		return StellarNetworkPubnet, true
	default:
		return "", false
	}
}

func settingsForStellarNetwork(value string) (stellarNetworkSettings, string, bool) {
	networkName, ok := canonicalStellarNetwork(value)
	if !ok {
		return stellarNetworkSettings{}, "", false
	}
	return stellarNetworkSettingsByNetwork[networkName], networkName, true
}

func isKnownHorizonHost(host string) bool {
	for _, settings := range stellarNetworkSettingsByNetwork {
		for _, knownHost := range settings.hosts {
			if strings.EqualFold(host, knownHost) {
				return true
			}
		}
	}
	return false
}

func horizonHostMatchesNetwork(host, networkName string) bool {
	settings, ok := stellarNetworkSettingsByNetwork[networkName]
	if !ok {
		return false
	}
	for _, knownHost := range settings.hosts {
		if strings.EqualFold(host, knownHost) {
			return true
		}
	}
	return false
}

func startConnectionPoolMonitor(db *sql.DB) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	connectionPoolMonitor.Lock()
	previousCancel := connectionPoolMonitor.cancel
	connectionPoolMonitor.cancel = cancel
	connectionPoolMonitor.done = done
	connectionPoolMonitor.Unlock()

	if previousCancel != nil {
		previousCancel()
	}

	go func() {
		defer close(done)
		MonitorConnectionPool(ctx, db)
	}()
}

// StopConnectionPoolMonitor waits for the pool monitor to exit before the
// database connection is closed during application shutdown.
func StopConnectionPoolMonitor() {
	connectionPoolMonitor.Lock()
	cancel := connectionPoolMonitor.cancel
	done := connectionPoolMonitor.done
	connectionPoolMonitor.cancel = nil
	connectionPoolMonitor.done = nil
	connectionPoolMonitor.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	<-done
}

type Config struct {
	Port              string
	Environment       string
	TLSEnabled        bool
	DatabaseURL       string
	ReadReplicaURL    string
	StellarNetwork    string
	StellarHorizonURL string
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
	// ExchangeRateCacheTTL is how long a rate fetched from the oracle is served
	// from Redis before the oracle is consulted again (EXCHANGE_RATE_CACHE_TTL,
	// a Go duration such as "60s"; default 60s).
	ExchangeRateCacheTTL time.Duration

	// Sandbox mode — when true the backend uses testnet defaults,
	// reduced fee rates, and relaxed validation suitable for
	// development and integration testing. Controlled by the
	// SANDBOX_MODE environment variable.
	SandboxMode bool

	// Batch remittance configuration
	MaxBatchSize int

	// Recurring remittance worker configuration (off by default)
	RecurringWorkerEnabled   bool
	RecurringWorkerInterval  time.Duration
	RecurringWorkerBatchSize int
	RecurringWorkerTimeout   time.Duration
	RecurringMaxFailures     int

	// CORS is configured with explicit origins; credentials are only sent
	// to exact origins listed in CORS_ALLOWED_ORIGINS.
	CORSAllowedOrigins   []string
	CORSAllowedMethods   []string
	CORSAllowedHeaders   []string
	CORSExposedHeaders   []string
	CORSAllowCredentials bool
	CORSMaxAge           time.Duration
}

// validateJWTSelfTest signs and verifies a test JWT token to ensure the key is functional at startup (#333).
func validateJWTSelfTest(secret string) error {
	if len(secret) < 32 {
		return fmt.Errorf("jwt secret must be at least 32 characters long, got %d", len(secret))
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "startup-selftest",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return fmt.Errorf("jwt signing self-test failed: %w", err)
	}

	parsed, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return fmt.Errorf("jwt verification self-test failed: %w", err)
	}
	return nil
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

	// Startup self-test for JWT secrets (#333)
	if err := validateJWTSelfTest(jwtSecret); err != nil {
		return nil, fmt.Errorf("JWT_SECRET self-test failed: %w", err)
	}
	if err := validateJWTSelfTest(jwtRefreshSecret); err != nil {
		return nil, fmt.Errorf("JWT_REFRESH_SECRET self-test failed: %w", err)
	}

	sandbox := getEnvOrDefault("SANDBOX_MODE", "false") == "true"

	corsAllowedOrigins := getEnvAsCSV("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"})
	corsAllowedMethods := getEnvAsCSV("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	corsAllowedHeaders := getEnvAsCSV("CORS_ALLOWED_HEADERS", []string{
		"Content-Type", "Authorization", "X-API-Version", "Accept-Version",
		"X-CSRF-Token", "X-Request-ID", "X-Correlation-ID",
	})
	corsExposedHeaders := getEnvAsCSV("CORS_EXPOSED_HEADERS", []string{
		"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset",
	})

	// Stellar network selection and Horizon endpoint. STELLAR_HORIZON_URL is
	// the canonical setting; HORIZON_URL remains a backwards-compatible alias.
	// The passphrase is derived from STELLAR_NETWORK so the same binary can be
	// pointed at testnet or pubnet at runtime.
	requestedNetwork := strings.TrimSpace(getEnvOrDefault("STELLAR_NETWORK", StellarNetworkTestnet))
	networkSettings, stellarNetwork, networkKnown := settingsForStellarNetwork(requestedNetwork)
	if !networkKnown {
		stellarNetwork = strings.ToLower(requestedNetwork)
	}
	horizonURL := strings.TrimSpace(getEnvOrDefault("STELLAR_HORIZON_URL", getEnvOrDefault("HORIZON_URL", networkSettings.horizonURL)))
	networkPassphrase := strings.TrimSpace(getEnvOrDefault("NETWORK_PASSPHRASE", networkSettings.passphrase))
	platformFeeBps := getEnvAsInt("PLATFORM_FEE_BPS", 50)
	forexFeeBps := getEnvAsInt("FOREX_FEE_BPS", 25)
	complianceFeeBps := getEnvAsInt("COMPLIANCE_FEE_BPS", 10)
	networkFeeBps := getEnvAsInt("NETWORK_FEE_BPS", 15)

	if sandbox {
		if !isEnvSet("STELLAR_NETWORK") {
			stellarNetwork = StellarNetworkTestnet
			networkSettings = stellarNetworkSettingsByNetwork[StellarNetworkTestnet]
		}
		if !isEnvSet("STELLAR_HORIZON_URL") && !isEnvSet("HORIZON_URL") {
			horizonURL = networkSettings.horizonURL
		}
		if !isEnvSet("NETWORK_PASSPHRASE") {
			networkPassphrase = networkSettings.passphrase
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
		ReadReplicaURL:    strings.TrimSpace(os.Getenv("DATABASE_READ_REPLICA_URL")),
		StellarNetwork:    stellarNetwork,
		StellarHorizonURL: horizonURL,
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

		ExchangeRateAPIURL:   getEnvOrDefault("EXCHANGE_RATE_API_URL", "https://open.er-api.com/v6/latest"),
		ExchangeRateCacheTTL: getEnvAsDuration("EXCHANGE_RATE_CACHE_TTL", 60*time.Second),

		SandboxMode: sandbox,

		MaxBatchSize: getEnvAsInt("MAX_BATCH_SIZE", 100),

		RecurringWorkerEnabled:   getEnvOrDefault("RECURRING_WORKER_ENABLED", "false") == "true",
		RecurringWorkerInterval:  getEnvAsDuration("RECURRING_WORKER_INTERVAL", 1*time.Minute),
		RecurringWorkerBatchSize: getEnvAsInt("RECURRING_WORKER_BATCH_SIZE", 50),
		RecurringWorkerTimeout:   getEnvAsDuration("RECURRING_WORKER_TIMEOUT", 30*time.Second),
		RecurringMaxFailures:     getEnvAsInt("RECURRING_MAX_FAILURES", 3),

		CORSAllowedOrigins:   corsAllowedOrigins,
		CORSAllowedMethods:   corsAllowedMethods,
		CORSAllowedHeaders:   corsAllowedHeaders,
		CORSExposedHeaders:   corsExposedHeaders,
		CORSAllowCredentials: getEnvOrDefault("CORS_ALLOW_CREDENTIALS", "false") == "true",
		CORSMaxAge:           getEnvAsDuration("CORS_MAX_AGE", 10*time.Minute),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	logger.Log.WithFields(map[string]interface{}{
		"stellar_network":    cfg.StellarNetwork,
		"horizon_url":        cfg.HorizonURL,
		"network_passphrase": cfg.NetworkPassphrase,
	}).Info("Active Stellar network configured")

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
	if replicaURL := strings.TrimSpace(c.ReadReplicaURL); replicaURL != "" && !strings.HasPrefix(replicaURL, "postgres://") && !strings.HasPrefix(replicaURL, "postgresql://") {
		errs = append(errs, "DATABASE_READ_REPLICA_URL must be a valid postgres connection URI (starting with postgres:// or postgresql://)")
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

	// 5. Stellar network, Horizon URL and passphrase consistency
	settings, networkName, networkKnown := settingsForStellarNetwork(c.StellarNetwork)
	if !networkKnown {
		errs = append(errs, fmt.Sprintf("STELLAR_NETWORK must be one of 'testnet', 'pubnet' (got %q)", c.StellarNetwork))
	} else {
		if passphrase := strings.TrimSpace(c.NetworkPassphrase); passphrase != "" && passphrase != settings.passphrase {
			errs = append(errs, fmt.Sprintf("NETWORK_PASSPHRASE does not match STELLAR_NETWORK=%q (expected %q)", networkName, settings.passphrase))
		}

		effectiveURL := strings.TrimSpace(c.StellarHorizonURL)
		if effectiveURL == "" {
			effectiveURL = strings.TrimSpace(c.HorizonURL)
		}
		parsedURL, parseErr := url.Parse(effectiveURL)
		if parseErr != nil || (strings.ToLower(parsedURL.Scheme) != "http" && strings.ToLower(parsedURL.Scheme) != "https") || strings.TrimSpace(parsedURL.Host) == "" {
			errs = append(errs, fmt.Sprintf("STELLAR_HORIZON_URL must be a valid HTTP/HTTPS URL (got %q); HORIZON_URL must be a valid HTTP/HTTPS URL (got %q)", c.StellarHorizonURL, c.HorizonURL))
		} else {
			host := parsedURL.Hostname()
			if isKnownHorizonHost(host) && !horizonHostMatchesNetwork(host, networkName) {
				errs = append(errs, fmt.Sprintf("STELLAR_HORIZON_URL host %q does not match STELLAR_NETWORK=%q (expected %s)", host, networkName, strings.Join(settings.hosts, " or ")))
			}
		}
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

	// 10. CORS Configuration
	if len(c.CORSAllowedOrigins) == 0 {
		errs = append(errs, "CORS_ALLOWED_ORIGINS must contain at least one origin")
	}
	for _, origin := range c.CORSAllowedOrigins {
		if err := validateCORSOrigin(origin); err != nil {
			errs = append(errs, fmt.Sprintf("CORS_ALLOWED_ORIGINS contains invalid origin %q: %v", origin, err))
		}
	}
	if len(c.CORSAllowedMethods) == 0 {
		errs = append(errs, "CORS_ALLOWED_METHODS must contain at least one method")
	}
	if len(c.CORSAllowedHeaders) == 0 {
		errs = append(errs, "CORS_ALLOWED_HEADERS must contain at least one header")
	}
	if c.CORSMaxAge <= 0 {
		errs = append(errs, "CORS_MAX_AGE must be positive")
	}
	if c.CORSAllowCredentials {
		corsValues := make([]string, 0, len(c.CORSAllowedOrigins)+len(c.CORSAllowedMethods)+len(c.CORSAllowedHeaders)+len(c.CORSExposedHeaders))
		corsValues = append(corsValues, c.CORSAllowedOrigins...)
		corsValues = append(corsValues, c.CORSAllowedMethods...)
		corsValues = append(corsValues, c.CORSAllowedHeaders...)
		corsValues = append(corsValues, c.CORSExposedHeaders...)
		for _, value := range corsValues {
			if value == "*" {
				errs = append(errs, "wildcard CORS values cannot be combined with credentials")
				break
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed with %d error(s):\n  - %s", len(errs), strings.Join(errs, "\n  - "))
	}
	return nil
}

// CORSConfig builds the gin-contrib/cors configuration from validated config
// values. Wildcards and origin functions are deliberately never used.
func (c *Config) CORSConfig() cors.Config {
	return cors.Config{
		AllowOrigins:     c.CORSAllowedOrigins,
		AllowMethods:     c.CORSAllowedMethods,
		AllowHeaders:     c.CORSAllowedHeaders,
		ExposeHeaders:    c.CORSExposedHeaders,
		AllowCredentials: c.CORSAllowCredentials,
		MaxAge:           c.CORSMaxAge,
	}
}

func validateCORSOrigin(origin string) error {
	if origin == "" || origin == "*" || strings.Contains(origin, "*") {
		return errors.New("must be an explicit origin")
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("origin must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("origin must contain only a scheme and host")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return errors.New("http origins are only allowed for localhost")
		}
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

	// Stop the monitor cleanly before closing the database pool.
	startConnectionPoolMonitor(sqlDB)

	return db, nil
}

// InitReadDB opens the optional read replica without making startup depend on
// it. Callers fall back to the primary when this returns nil.
func InitReadDB(cfg *Config) (*gorm.DB, error) {
	if strings.TrimSpace(cfg.ReadReplicaURL) == "" {
		return nil, nil
	}
	db, err := gorm.Open(postgres.Open(cfg.ReadReplicaURL), &gorm.Config{Logger: newGormLogger(cfg.Environment)})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to read replica: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get read replica sql.DB: %w", err)
	}
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	return db, nil
}

// MonitorConnectionPool monitors database connection pool stats and logs alerts
func MonitorConnectionPool(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := db.Stats()

			// Log connection pool metrics
			logger.Log.WithFields(map[string]interface{}{
				"open_connections":    stats.OpenConnections,
				"in_use":              stats.InUse,
				"idle":                stats.Idle,
				"wait_count":          stats.WaitCount,
				"wait_duration_ms":    stats.WaitDuration.Milliseconds(),
				"max_idle_closed":     stats.MaxIdleClosed,
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
					"wait_count":    stats.WaitCount,
					"wait_duration": stats.WaitDuration.Seconds(),
					"connections":   stats.OpenConnections,
				}).Warn("Queries waiting for database connections")
			}
		}
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsCSV(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
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

// getEnvAsDuration parses a Go duration ("30s", "2m"). Unparseable or
// non-positive values fall back to the default rather than failing startup.
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil || value <= 0 {
		return defaultValue
	}
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
// (even to an empty string). This distinguishes "not set" from "set to ”".
func isEnvSet(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

// ResetSandbox reverts a Config instance to sandbox defaults in-place. This is
// useful for tests or runtime switches that need to restore sandbox-friendly
// values after a temporary override.
func (c *Config) ResetSandbox() {
	c.SandboxMode = true
	c.StellarNetwork = StellarNetworkTestnet
	c.StellarHorizonURL = "https://horizon-testnet.stellar.org"
	c.HorizonURL = "https://horizon-testnet.stellar.org"
	c.NetworkPassphrase = network.TestNetworkPassphrase
	c.PlatformFeeBps = 10
	c.ForexFeeBps = 5
	c.ComplianceFeeBps = 0
	c.NetworkFeeBps = 0
	c.MinFee = 0
	c.MaxFee = 0
}
