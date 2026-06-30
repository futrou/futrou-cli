package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// EncPrefix marks a token value as machine-encrypted in the JSON file,
// distinguishing it from legacy plaintext tokens written by older CLI versions.
const EncPrefix = "enc:v1:"

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

// DeriveKey hashes the three fingerprint components into a 32-byte AES-256 key.
// Exported so tests can verify that changing any single component produces a
// different key (a token encrypted under one identity cannot be decrypted under another).
func DeriveKey(cliId, machineId, userId string) []byte {
	sum := sha256.Sum256([]byte(cliId + ":" + machineId + ":" + userId))
	return sum[:]
}

// DeviceFingerprint derives an AES key from the CLI name, machine ID, and OS
// user ID. MachineID may be empty in minimal containers or restricted
// environments — the token is still encrypted, just without machine binding.
func DeviceFingerprint(cliName string) []byte {
	return DeriveKey(cliName, MachineID(), strconv.Itoa(os.Getuid()))
}

// EncryptToken encrypts plaintext with the device fingerprint key.
// Always produces an enc:v1:-prefixed value — never stores as plaintext.
func EncryptToken(plaintext, cliName string) string {
	if plaintext == "" {
		return ""
	}
	return EncryptWithKey(plaintext, DeviceFingerprint(cliName))
}

// DecryptToken reverses EncryptToken. Values without the enc:v1: prefix are
// treated as legacy plaintext and returned as-is.
func DecryptToken(stored, cliName string) (string, error) {
	if stored == "" {
		return "", nil
	}
	rest, ok := stripPrefix(stored, EncPrefix)
	if !ok {
		return stored, nil // legacy plaintext
	}
	return decryptBase64Payload(rest, DeviceFingerprint(cliName))
}

// EncryptWithKey encrypts plaintext with an explicit key, returning an enc:v1:-prefixed value.
// Used by tests to exercise key isolation without relying on the real device fingerprint.
func EncryptWithKey(plaintext string, key []byte) string {
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
	return EncPrefix + base64.StdEncoding.EncodeToString(sealed)
}

// DecryptWithKey decrypts an enc:v1:-prefixed value with an explicit key.
// Used by tests to exercise key isolation without relying on the real device fingerprint.
func DecryptWithKey(stored string, key []byte) (string, error) {
	rest, ok := stripPrefix(stored, EncPrefix)
	if !ok {
		return stored, nil
	}
	return decryptBase64Payload(rest, key)
}

func decryptBase64Payload(b64 string, key []byte) (string, error) {
	sealed, err := base64.StdEncoding.DecodeString(b64)
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
		return "", errors.New("token was encrypted on a different device")
	}
	return string(plaintext), nil
}

// MachineID returns a stable identifier for the current device.
// It is not secret — any local process can read it — but it ties an
// encrypted token to the device it was created on: copying the config
// file to another machine yields a different key and decryption fails.
// Returns "" if no stable identifier could be found.
func MachineID() string {
	switch runtime.GOOS {
	case "linux":
		for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if data, err := os.ReadFile(path); err == nil {
				if id := strings.TrimSpace(string(data)); id != "" {
					return id
				}
			}
		}
	case "darwin":
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "IOPlatformUUID") {
					parts := strings.Split(line, "\"")
					if len(parts) >= 4 {
						return parts[3]
					}
				}
			}
		}
	case "windows":
		out, err := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
		if err == nil {
			for _, field := range strings.Fields(string(out)) {
				if strings.Count(field, "-") == 4 && len(field) == 36 {
					return field
				}
			}
		}
	}
	return ""
}

func stripPrefix(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || s[:len(prefix)] != prefix {
		return "", false
	}
	return s[len(prefix):], true
}
