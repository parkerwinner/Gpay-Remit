package validators

import "testing"

func TestValidateStellarAddress(t *testing.T) {
	err := ValidateStellarAddress("GABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789ABCDEFGHIJKLMNOPQRS")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
}
