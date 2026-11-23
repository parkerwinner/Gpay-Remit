package validators

import (
	"errors"
	"strings"
)

func ValidateStellarAddress(addr string) error {
	if !strings.HasPrefix(addr, "G") {
		return errors.New("must start with G")
	}
	if len(addr) != 56 {
		return errors.New("invalid length")
	}
	// Simplified verification
	return nil
}

func ValidateAmount(amt float64) error {
	if amt <= 0 {
		return errors.New("amount must be positive")
	}
	return nil
}

// SupportedCurrencies is the canonical list of currencies supported by the platform.
var SupportedCurrencies = []string{"USD", "EUR", "GBP", "XLM", "USDC", "NGN", "KES", "GHS"}

// IsSupportedCurrency reports whether code is a supported currency (case-insensitive).
func IsSupportedCurrency(code string) bool {
	upper := strings.ToUpper(strings.TrimSpace(code))
	for _, c := range SupportedCurrencies {
		if c == upper {
			return true
		}
	}
	return false
}

func ValidateCurrency(currency string) error {
	if strings.TrimSpace(currency) == "" {
		return errors.New("currency is required")
	}
	if !IsSupportedCurrency(currency) {
		return errors.New("unsupported currency")
	}
	return nil
}

func ValidateBusinessRules(sender, recipient string) error {
	if sender == recipient {
		return errors.New("sender and recipient cannot be the same")
	}
	return nil
}
