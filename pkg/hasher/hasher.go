// Package hasher contains reusable hashing helpers.
package hasher

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Bytes calculates a SHA-256 hash for a byte slice and returns hex output.
func SHA256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SHA256String calculates a SHA-256 hash for a string value.
func SHA256String(value string) string {
	return SHA256Bytes([]byte(value))
}

// VerifySHA256String compares the SHA-256 hash of a string with an expected value.
func VerifySHA256String(value, expected string) bool {
	return SHA256String(value) == expected
}

// HMACSHA256String calculates a keyed SHA-256 HMAC for a string value.
func HMACSHA256String(value, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// HashString calculates HMAC-SHA256 when key is provided and plain SHA-256 otherwise.
func HashString(value, key string) string {
	if key == "" {
		return SHA256String(value)
	}
	return HMACSHA256String(value, key)
}
