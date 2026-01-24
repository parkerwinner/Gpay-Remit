package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/yourusername/gpay-remit/models"
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
	}, nil
}

func InitDB(cfg *Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.Payment{}, &models.Invoice{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
