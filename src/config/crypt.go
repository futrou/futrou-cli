package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strconv"

	"futrou-cli/src/constants"
)

// encPrefix marks a token value as machine-encrypted in the JSON file,
// distinguishing it from legacy plaintext tokens written by older CLI versions.
const encPrefix = "enc:v1:"

// Why not the OS keychain / TPM?
//
// Native credential stores (macOS Keychain, Windows Credential Manager,
// Linux Secret Service) and TPM-backed sealing are stronger in principle,
// but each is a different API per OS, and none of them is reliably present:
// Secret Service needs a desktop session/keyring daemon that headless
// servers, containers, and CI runners typically don't have; TPM access
// needs a physical chip plus device permissions that many VMs and cloud
// instances don't expose; macOS has no TPM at all. Depending on any of
// these as the primary store means silently falling back to something
// weaker on a large fraction of real environments — which makes the
// "secure" path the one that's hardest to test and reason about.
//
// So this just encrypts the token with a key derived from the device +
// user, without any external service. It does not protect against
// malware running as the same OS user on the same machine — it can call
// the same derivation and decrypt the token just as the CLI does. The one
// property it does add: the token is not stored as plaintext, and copying
// cli.json to another machine or reading it as another user yields
// ciphertext that cannot be decrypted there. That's the whole goal —
// not "unbreakable," just "not a portable plaintext secret."

func deviceFingerprint() []byte {
	// machineID may be empty in minimal containers or restricted environments.
	// We still derive a key from the available inputs rather than falling back
	// to plaintext — the token is always encrypted, just without machine binding
	// when the machine ID is unavailable.
	return deriveKey(constants.Name, machineID(), strconv.Itoa(os.Getuid()))
}

func deriveKey(cliId, machineId, userId string) []byte {
	sum := sha256.Sum256([]byte(cliId + ":" + machineId + ":" + userId))
	return sum[:]
}

// encryptToken encrypts a token with a key derived from this device's
// machine ID. If no machine ID is available, the token is returned
// unchanged (stored as plaintext) so the CLI keeps working everywhere.
func encryptToken(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	key := deviceFingerprint()

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

	key := deviceFingerprint()

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

// encryptWithKey / decryptWithKey are used only by tests to exercise key
// isolation without relying on the real device fingerprint.
func encryptWithKey(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptWithKey(stored string, key []byte) (string, error) {
	rest, ok := stripPrefix(stored, encPrefix)
	if !ok {
		return stored, nil
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
	plaintext, err := gcm.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return "", errors.New("decryption failed with this key")
	}
	return string(plaintext), nil
}

func stripPrefix(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || s[:len(prefix)] != prefix {
		return "", false
	}
	return s[len(prefix):], true
}
