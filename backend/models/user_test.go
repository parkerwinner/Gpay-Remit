package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword_GeneratesHash(t *testing.T) {
	hash, err := HashPassword("Secure@Pass1")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "Secure@Pass1", hash)
}

func TestHashPassword_Uniqueness(t *testing.T) {
	// bcrypt generates a unique salt each time — same input must produce different hashes
	hash1, err := HashPassword("Secure@Pass1")
	require.NoError(t, err)
	hash2, err := HashPassword("Secure@Pass1")
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)
}

func TestHashPassword_WeakPasswordRejected(t *testing.T) {
	_, err := HashPassword("weak")
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "password")
}

func TestComparePassword_Valid(t *testing.T) {
	hash, err := HashPassword("Secure@Pass1")
	require.NoError(t, err)
	assert.True(t, ComparePassword(hash, "Secure@Pass1"))
}

func TestComparePassword_Invalid(t *testing.T) {
	hash, err := HashPassword("Secure@Pass1")
	require.NoError(t, err)
	assert.False(t, ComparePassword(hash, "WrongPass@1"))
	assert.False(t, ComparePassword(hash, ""))
	assert.False(t, ComparePassword(hash, "secure@pass1"))
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{"valid password with special char", "Secure@Pass1", false, ""},
		{"minimum valid", "Abc#9876Z", false, ""},
		{"too short", "Ab@1", true, "8 characters"},
		{"no uppercase", "secure@pass1", true, "uppercase"},
		{"no lowercase", "SECURE@PASS1", true, "lowercase"},
		{"no digit", "Secure@Passwd", true, "digit"},
		{"no special char", "SecurePass1", true, "special character"},
		{"empty string", "", true, "8 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUser_EncryptedSensitiveFields(t *testing.T) {
	u := User{
		Email:             "alice@example.com",
		Name:              "Alice",
		SSN:               "123-45-6789",
		BankAccountNumber: "US9876543210123456",
		TaxID:             "TAX-998877",
	}

	assert.Equal(t, "123-45-6789", u.SSN.String())
	assert.Equal(t, "US9876543210123456", u.BankAccountNumber.String())
	assert.Equal(t, "TAX-998877", u.TaxID.String())

	// Value() -> encrypted for database storage
	ssnVal, err := u.SSN.Value()
	require.NoError(t, err)
	assert.NotEqual(t, "123-45-6789", ssnVal)
	assert.NotEmpty(t, ssnVal)

	bankVal, err := u.BankAccountNumber.Value()
	require.NoError(t, err)
	assert.NotEqual(t, "US9876543210123456", bankVal)
	assert.NotEmpty(t, bankVal)
}
