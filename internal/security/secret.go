// Package security contains helpers for protecting persisted secrets.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const encryptedStringPrefix = "enc:v1:"

// StringCipher encrypts short string secrets before they are persisted.
type StringCipher struct {
	gcm cipher.AEAD
}

// NewStringCipher creates an AES-GCM cipher from an application secret.
func NewStringCipher(secret string) (*StringCipher, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, nil
	}

	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("security: create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("security: create gcm cipher: %w", err)
	}
	return &StringCipher{gcm: gcm}, nil
}

// Encrypt returns an encrypted representation of value.
func (c *StringCipher) Encrypt(value string) (string, error) {
	if c == nil || value == "" || strings.HasPrefix(value, encryptedStringPrefix) {
		return value, nil
	}

	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("security: create nonce: %w", err)
	}
	ciphertext := c.gcm.Seal(nil, nonce, []byte(value), nil)

	payload := make([]byte, 0, len(nonce)+len(ciphertext))
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)
	return encryptedStringPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

// Decrypt returns plaintext for encrypted values and passes legacy plaintext through.
func (c *StringCipher) Decrypt(value string) (string, error) {
	if c == nil || value == "" || !strings.HasPrefix(value, encryptedStringPrefix) {
		return value, nil
	}

	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encryptedStringPrefix))
	if err != nil {
		return "", fmt.Errorf("security: decode encrypted value: %w", err)
	}
	if len(payload) < c.gcm.NonceSize() {
		return "", fmt.Errorf("security: encrypted value is too short")
	}

	nonce := payload[:c.gcm.NonceSize()]
	ciphertext := payload[c.gcm.NonceSize():]
	plaintext, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("security: decrypt value: %w", err)
	}
	return string(plaintext), nil
}
