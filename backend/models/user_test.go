package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword_GeneratesHash(t *testing.T) {
	hash, err := HashPassword("SecurePass1")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "SecurePass1", hash)
}

func TestHashPassword_Uniqueness(t *testing.T) {
	// bcrypt generates a unique salt each time — same input must produce different hashes
	hash1, err := HashPassword("SecurePass1")
	require.NoError(t, err)
	hash2, err := HashPassword("SecurePass1")
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)
}

func TestHashPassword_WeakPasswordRejected(t *testing.T) {
	_, err := HashPassword("weak")
	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "password")
}

func TestComparePassword_Valid(t *testing.T) {
	hash, err := HashPassword("SecurePass1")
	require.NoError(t, err)
	assert.True(t, ComparePassword(hash, "SecurePass1"))
}

func TestComparePassword_Invalid(t *testing.T) {
	hash, err := HashPassword("SecurePass1")
	require.NoError(t, err)
	assert.False(t, ComparePassword(hash, "WrongPass1"))
	assert.False(t, ComparePassword(hash, ""))
	assert.False(t, ComparePassword(hash, "securepass1"))
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid password", "SecurePass1", false},
		{"minimum 8 chars", "SecPas1X", false},
		{"too short", "Ab1", true},
		{"no uppercase", "securepass1", true},
		{"no lowercase", "SECUREPASS1", true},
		{"no digit", "SecurePasswd", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tt.password)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
