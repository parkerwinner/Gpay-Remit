package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	Port              string
	DatabaseURL       string
	StellarNetwork    string
	HorizonURL        string
	ContractID        string
	EscrowContractID  string
	NetworkPassphrase string
	JWTSecret         string
	JWTRefreshSecret  string

	// Database connection pool settings
	DBMaxIdleConns    int
	DBMaxOpenConns    int
	DBConnMaxLifetime time.Duration
}

func LoadConfig() (*Config, error) {
	godotenv.Load()

	return &Config{
		Port:              os.Getenv("PORT"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		StellarNetwork:    getEnvOrDefault("STELLAR_NETWORK", "testnet"),
		HorizonURL:        getEnvOrDefault("HORIZON_URL", "https://horizon-testnet.stellar.org"),
		ContractID:        os.Getenv("CONTRACT_ID"),
		EscrowContractID:  os.Getenv("ESCROW_CONTRACT_ID"),
		NetworkPassphrase: getEnvOrDefault("NETWORK_PASSPHRASE", "Test SDF Network ; September 2015"),
		JWTSecret:         getEnvOrDefault("JWT_SECRET", "super-secret-key-change-me"),
		JWTRefreshSecret:  getEnvOrDefault("JWT_REFRESH_SECRET", "super-secret-refresh-key-change-me"),

		DBMaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 10),
		DBMaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 100),
		DBConnMaxLifetime: time.Duration(getEnvAsInt("DB_CONN_MAX_LIFETIME_MIN", 60)) * time.Minute,
	}, nil
}

func InitDB(cfg *Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
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

	return db, nil
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
