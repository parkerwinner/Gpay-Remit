package encryption

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	ErrInvalidKey        = errors.New("encryption: key must be 32 bytes for AES-256")
	ErrCiphertextTooShort = errors.New("encryption: ciphertext is too short")
	ErrDecryptionFailed  = errors.New("encryption: failed to decrypt / authentication failed")
)

// Encryptor defines the interface for data encryption and decryption at rest.
type Encryptor interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
	EncryptString(ctx context.Context, plaintext string) (string, error)
	DecryptString(ctx context.Context, ciphertext string) (string, error)
}

// AESGCMEncryptor implements Encryptor using AES-256-GCM.
type AESGCMEncryptor struct {
	aead cipher.AEAD
}

// NewAESGCMEncryptor creates a new AES-256-GCM encryptor from a 32-byte key.
// If the key is not 32 bytes, it derives a 32-byte key using SHA-256.
func NewAESGCMEncryptor(key []byte) (*AESGCMEncryptor, error) {
	if len(key) == 0 {
		return nil, errors.New("encryption: key cannot be empty")
	}

	derivedKey := key
	if len(key) != 32 {
		h := sha256.Sum256(key)
		derivedKey = h[:]
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM cipher: %w", err)
	}

	return &AESGCMEncryptor{aead: aead}, nil
}

// Encrypt encrypts plaintext using AES-GCM and prepends a random 12-byte nonce.
func (e *AESGCMEncryptor) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := e.aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-GCM, extracting the prepended nonce.
func (e *AESGCMEncryptor) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	nonceSize := e.aead.NonceSize()
	if len(ciphertext) < nonceSize+e.aead.Overhead() {
		return nil, ErrCiphertextTooShort
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.aead.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// EncryptString encrypts a string and returns a base64-encoded representation.
func (e *AESGCMEncryptor) EncryptString(ctx context.Context, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	encrypted, err := e.Encrypt(ctx, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// DecryptString decodes a base64 string and decrypts it.
func (e *AESGCMEncryptor) DecryptString(ctx context.Context, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decoding base64 ciphertext: %w", err)
	}
	decrypted, err := e.Decrypt(ctx, decoded)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

// Global default encryptor for GORM model fields
var (
	defaultEncryptor Encryptor
	encryptorMu      sync.RWMutex
)

func init() {
	// Initialize default encryptor with ENCRYPTION_KEY or a fallback key
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		key = "gpay-remit-default-master-key-for-at-rest-encryption-32"
	}
	enc, err := NewAESGCMEncryptor([]byte(key))
	if err == nil {
		defaultEncryptor = enc
	}
}

// SetDefaultEncryptor configures the global encryptor used by model fields.
func SetDefaultEncryptor(e Encryptor) {
	encryptorMu.Lock()
	defer encryptorMu.Unlock()
	defaultEncryptor = e
}

// GetDefaultEncryptor returns the global default encryptor.
func GetDefaultEncryptor() Encryptor {
	encryptorMu.RLock()
	defer encryptorMu.RUnlock()
	return defaultEncryptor
}

// EncryptedString is a custom GORM / SQL type that automatically encrypts
// sensitive data at rest in PostgreSQL and decrypts it upon retrieval.
type EncryptedString string

// Value implements the driver.Valuer interface for database writes.
func (es EncryptedString) Value() (driver.Value, error) {
	str := string(es)
	if str == "" {
		return "", nil
	}

	enc := GetDefaultEncryptor()
	if enc == nil {
		return str, nil
	}

	return enc.EncryptString(context.Background(), str)
}

// Scan implements the sql.Scanner interface for database reads.
func (es *EncryptedString) Scan(value interface{}) error {
	if value == nil {
		*es = ""
		return nil
	}

	var rawStr string
	switch v := value.(type) {
	case string:
		rawStr = v
	case []byte:
		rawStr = string(v)
	default:
		return fmt.Errorf("cannot scan %T into EncryptedString", value)
	}

	if rawStr == "" {
		*es = ""
		return nil
	}

	enc := GetDefaultEncryptor()
	if enc == nil {
		*es = EncryptedString(rawStr)
		return nil
	}

	decrypted, err := enc.DecryptString(context.Background(), rawStr)
	if err != nil {
		// If decryption fails (e.g. legacy unencrypted data during migration), fall back to raw
		*es = EncryptedString(rawStr)
		return nil
	}

	*es = EncryptedString(decrypted)
	return nil
}

// String returns the underlying plaintext string.
func (es EncryptedString) String() string {
	return string(es)
}
