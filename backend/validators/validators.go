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

func ValidateCurrency(currency string) error {
	if currency == "" {
		return errors.New("currency is required")
	}
	return nil
}

func ValidateBusinessRules(sender, recipient string) error {
	if sender == recipient {
		return errors.New("sender and recipient cannot be the same")
	}
	return nil
}
