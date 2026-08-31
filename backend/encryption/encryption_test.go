package encryption

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAESGCMEncryptor_EncryptDecrypt(t *testing.T) {
	key := []byte("01234567890123456789012345678901") // 32 bytes
	enc, err := NewAESGCMEncryptor(key)
	require.NoError(t, err)

	ctx := context.Background()
	plaintext := "123-45-6789" // SSN

	ciphertext, err := enc.Encrypt(ctx, []byte(plaintext))
	require.NoError(t, err)
	assert.NotEqual(t, []byte(plaintext), ciphertext)

	decrypted, err := enc.Decrypt(ctx, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(decrypted))
}

func TestAESGCMEncryptor_EncryptString_DecryptString(t *testing.T) {
	key := []byte("super-secret-key-passphrase")
	enc, err := NewAESGCMEncryptor(key)
	require.NoError(t, err)

	ctx := context.Background()
	bankAccount := "US1234567890123456"

	encryptedStr, err := enc.EncryptString(ctx, bankAccount)
	require.NoError(t, err)
	assert.NotEmpty(t, encryptedStr)
	assert.NotEqual(t, bankAccount, encryptedStr)

	decryptedStr, err := enc.DecryptString(ctx, encryptedStr)
	require.NoError(t, err)
	assert.Equal(t, bankAccount, decryptedStr)

	// Empty string roundtrip
	emptyEnc, err := enc.EncryptString(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "", emptyEnc)

	emptyDec, err := enc.DecryptString(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "", emptyDec)
}

func TestAESGCMEncryptor_TamperedCiphertextFails(t *testing.T) {
	key := []byte("a-secure-32-byte-encryption-key!")
	enc, err := NewAESGCMEncryptor(key)
	require.NoError(t, err)

	ctx := context.Background()
	ciphertext, err := enc.Encrypt(ctx, []byte("sensitive-data"))
	require.NoError(t, err)

	// Tamper with one byte in the ciphertext payload
	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err = enc.Decrypt(ctx, ciphertext)
	require.Error(t, err)
	assert.Equal(t, ErrDecryptionFailed, err)
}

func TestEncryptedString_GORMDriverValuerAndScanner(t *testing.T) {
	key := []byte("gpay-remit-master-key-for-test-32")
	enc, err := NewAESGCMEncryptor(key)
	require.NoError(t, err)
	SetDefaultEncryptor(enc)

	ssn := EncryptedString("987-65-4321")

	// Value() -> DB write
	val, err := ssn.Value()
	require.NoError(t, err)
	dbValue, ok := val.(string)
	require.True(t, ok)
	assert.NotEmpty(t, dbValue)
	assert.NotEqual(t, "987-65-4321", dbValue)

	// Scan() -> DB read
	var scanned EncryptedString
	err = scanned.Scan(dbValue)
	require.NoError(t, err)
	assert.Equal(t, "987-65-4321", scanned.String())

	// Scan() from nil
	var nilScanned EncryptedString
	err = nilScanned.Scan(nil)
	require.NoError(t, err)
	assert.Equal(t, "", nilScanned.String())
}
