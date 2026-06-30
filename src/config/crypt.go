package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// encPrefix marks a token value as machine-encrypted in the JSON file,
// distinguishing it from legacy plaintext tokens written by older CLI versions.
const encPrefix = "enc:v1:"

func deviceKey() ([]byte, bool) {
	id := machineID()
	if id == "" {
		return nil, false
	}
	// Bind the key to this specific app, not just the device, so the
	// derived key isn't reusable by unrelated software reading the same ID.
	sum := sha256.Sum256([]byte("futrou-cli:" + id))
	return sum[:], true
}

// encryptToken encrypts a token with a key derived from this device's
// machine ID. If no machine ID is available, the token is returned
// unchanged (stored as plaintext) so the CLI keeps working everywhere.
func encryptToken(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	key, ok := deviceKey()
	if !ok {
		return plaintext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return plaintext
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return plaintext
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plaintext
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed)
}

// decryptToken reverses encryptToken. Values without the enc:v1: prefix
// are treated as legacy plaintext and returned as-is.
func decryptToken(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	rest, ok := stripPrefix(stored, encPrefix)
	if !ok {
		return stored, nil // legacy plaintext
	}

	key, ok := deviceKey()
	if !ok {
		return "", errors.New("no machine identifier available to decrypt token")
	}

	sealed, err := base64.StdEncoding.DecodeString(rest)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return "", errors.New("malformed encrypted token")
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("token was encrypted on a different device")
	}
	return string(plaintext), nil
}

func stripPrefix(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || s[:len(prefix)] != prefix {
		return "", false
	}
	return s[len(prefix):], true
}
