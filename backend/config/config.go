package config

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
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

func RunMigrations(databaseURL string) error {
	m, err := migrate.New(
		"file://migrations",
		databaseURL,
	)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Println("Database migrations completed successfully")
	return nil
}

func InitDB(cfg *Config) (*gorm.DB, error) {
	// Run migrations first
	if err := RunMigrations(cfg.DatabaseURL); err != nil {
		return nil, err
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		PrepareStmt:            true, // Use prepared statements for all queries
		SkipDefaultTransaction: true, // Optimize performance by skipping default transactions
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
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
