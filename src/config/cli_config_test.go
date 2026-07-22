package config

import (
	"os"
	"path/filepath"
	"testing"

	"futrou-cli/src/utils"
)

// helpers

func tempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("FUTROU_API_URL", "")
	t.Setenv("FUTROU_API_TOKEN", "")
	return dir
}

func cfgPath(home string) string {
	return filepath.Join(home, ".futrou", "cli.json")
}

func writeRaw(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".futrou")
	os.MkdirAll(dir, 0700)
	if err := os.WriteFile(cfgPath(home), []byte(content), 0600); err != nil {
		t.Fatalf("writeRaw: %v", err)
	}
}

// ── encryptToken / decryptToken ──────────────────────────────────────────────

func TestEncryptDecrypt_roundTrip(t *testing.T) {
	plain := "super-secret-api-key"
	enc := encryptToken(plain)

	// encrypted value must always carry the prefix — never stored as plaintext
	if !hasPrefix(enc, utils.EncPrefix) {
		t.Fatalf("encrypted value missing prefix: %q", enc)
	}

	got, err := decryptToken(enc)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if got != plain {
		t.Errorf("got %q, want %q", got, plain)
	}
}

func TestEncryptToken_alwaysEncrypted(t *testing.T) {
	// Token must never be stored as plaintext, even when machine ID is unavailable.
	// DeviceFingerprint falls back to cliId+userId only, but still encrypts.
	enc := encryptToken("any-token")
	if !hasPrefix(enc, utils.EncPrefix) {
		t.Errorf("token stored as plaintext (missing enc:v1: prefix): %q", enc)
	}
}

func TestEncryptDecrypt_emptyString(t *testing.T) {
	if enc := encryptToken(""); enc != "" {
		t.Errorf("encryptToken(\"\") = %q, want \"\"", enc)
	}
	got, err := decryptToken("")
	if err != nil || got != "" {
		t.Errorf("decryptToken(\"\") = %q, %v; want \"\", nil", got, err)
	}
}

func TestEncryptDecrypt_uniqueEachTime(t *testing.T) {
	// Different nonces mean the same plaintext encrypts differently each call.
	a := encryptToken("token")
	b := encryptToken("token")
	if a == b {
		t.Error("two encryptions of the same token produced identical output (random nonce broken?)")
	}
}

func TestDecrypt_legacyPlaintext(t *testing.T) {
	// Values without the enc:v1: prefix are legacy plaintext — returned as-is.
	plain := "old-plaintext-token"
	got, err := decryptToken(plain)
	if err != nil || got != plain {
		t.Errorf("decryptToken(legacy) = %q, %v; want %q, nil", got, err, plain)
	}
}

func TestDecrypt_corruptedCiphertext(t *testing.T) {
	corrupted := utils.EncPrefix + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
	_, err := decryptToken(corrupted)
	if err == nil {
		t.Error("expected error decrypting corrupted ciphertext, got nil")
	}
}

func TestDecrypt_malformedBase64(t *testing.T) {
	_, err := decryptToken(utils.EncPrefix + "!!!not-base64!!!")
	if err == nil {
		t.Error("expected error for malformed base64, got nil")
	}
}

func TestDecrypt_tooShortPayload(t *testing.T) {
	// Valid base64 but fewer bytes than the AES-GCM nonce size (12 bytes).
	_, err := decryptToken(utils.EncPrefix + "dG9v") // "too" — only 3 bytes
	if err == nil {
		t.Error("expected error for too-short payload, got nil")
	}
}

// ── TokenFor / SetToken ──────────────────────────────────────────────────────

func TestSetToken_getToken_roundTrip(t *testing.T) {
	tempHome(t)
	cfg := &Config{}
	cfg.SetToken("https://api.futrou.com", "tok-123")

	got := cfg.TokenFor("https://api.futrou.com")
	if got != "tok-123" {
		t.Errorf("TokenFor = %q, want %q", got, "tok-123")
	}
}

func TestTokenFor_urlNormalization(t *testing.T) {
	tempHome(t)
	cfg := &Config{}
	cfg.SetToken("https://api.futrou.com", "tok-abc")

	// Trailing slash, /v2 suffix, and mixed case should all resolve to the same token.
	for _, variant := range []string{
		"https://api.futrou.com/",
		"https://api.futrou.com/v2",
		"https://api.futrou.com/v2/",
		"HTTPS://API.FUTROU.COM",
	} {
		got := cfg.TokenFor(variant)
		if got != "tok-abc" {
			t.Errorf("TokenFor(%q) = %q, want %q", variant, got, "tok-abc")
		}
	}
}

func TestSetToken_perURLIsolation(t *testing.T) {
	tempHome(t)
	cfg := &Config{}
	cfg.SetToken("https://api.futrou.com", "prod-tok")
	cfg.SetToken("https://staging.futrou.com", "staging-tok")

	if got := cfg.TokenFor("https://api.futrou.com"); got != "prod-tok" {
		t.Errorf("prod token = %q, want %q", got, "prod-tok")
	}
	if got := cfg.TokenFor("https://staging.futrou.com"); got != "staging-tok" {
		t.Errorf("staging token = %q, want %q", got, "staging-tok")
	}
}

func TestTokenFor_missingURL(t *testing.T) {
	tempHome(t)
	cfg := &Config{}
	if got := cfg.TokenFor("https://api.futrou.com"); got != "" {
		t.Errorf("TokenFor unknown url = %q, want \"\"", got)
	}
}

func TestTokenFor_nilTokens(t *testing.T) {
	cfg := &Config{ApiTokens: nil}
	if got := cfg.TokenFor("https://api.futrou.com"); got != "" {
		t.Errorf("TokenFor on nil Tokens = %q, want \"\"", got)
	}
}

func TestSetToken_overwrite(t *testing.T) {
	tempHome(t)
	cfg := &Config{}
	cfg.SetToken("https://api.futrou.com", "first")
	cfg.SetToken("https://api.futrou.com", "second")

	if got := cfg.TokenFor("https://api.futrou.com"); got != "second" {
		t.Errorf("after overwrite TokenFor = %q, want %q", got, "second")
	}
}

// ── Load ─────────────────────────────────────────────────────────────────────

func TestLoad_noFile(t *testing.T) {
	tempHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with no file: %v", err)
	}
	if cfg.ApiUrl == "" {
		t.Error("ApiUrl should default to non-empty when no config file exists")
	}
}

func TestLoad_corruptJSON(t *testing.T) {
	home := tempHome(t)
	writeRaw(t, home, "{not valid json!!")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with corrupt JSON should not error: %v", err)
	}
	if got := cfg.TokenFor(cfg.ApiUrl); got != "" {
		t.Errorf("corrupt file should yield no token, got %q", got)
	}
}

func TestLoad_envApiUrlOverride(t *testing.T) {
	home := tempHome(t)
	writeRaw(t, home, `{"apiUrl":"https://api.futrou.com"}`)
	t.Setenv("FUTROU_API_URL", "https://custom.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ApiUrl != "https://custom.example.com" {
		t.Errorf("ApiUrl = %q, want https://custom.example.com", cfg.ApiUrl)
	}
}

func TestLoad_envApiTokenOverride(t *testing.T) {
	home := tempHome(t)
	writeRaw(t, home, `{"apiUrl":"https://api.futrou.com"}`)
	t.Setenv("FUTROU_API_TOKEN", "env-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.TokenFor(cfg.ApiUrl); got != "env-token" {
		t.Errorf("token from env = %q, want %q", got, "env-token")
	}
}

// ── Save / Load round-trip ───────────────────────────────────────────────────

func TestSaveLoad_roundTrip(t *testing.T) {
	tempHome(t)

	cfg := &Config{ApiUrl: "https://api.futrou.com"}
	cfg.SetToken("https://api.futrou.com", "round-trip-token")
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg2.TokenFor(cfg2.ApiUrl); got != "round-trip-token" {
		t.Errorf("after save/load token = %q, want %q", got, "round-trip-token")
	}
}

func TestSaveLoad_multipleURLs(t *testing.T) {
	tempHome(t)

	cfg := &Config{ApiUrl: "https://api.futrou.com"}
	cfg.SetToken("https://api.futrou.com", "prod-token")
	cfg.SetToken("https://staging.futrou.com", "staging-token")
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg2.TokenFor("https://api.futrou.com"); got != "prod-token" {
		t.Errorf("prod token = %q, want %q", got, "prod-token")
	}
	if got := cfg2.TokenFor("https://staging.futrou.com"); got != "staging-token" {
		t.Errorf("staging token = %q, want %q", got, "staging-token")
	}
}

func TestSaveLoad_encryptedOnDisk(t *testing.T) {
	home := tempHome(t)

	cfg := &Config{ApiUrl: "https://api.futrou.com"}
	cfg.SetToken("https://api.futrou.com", "secret")
	Save(cfg)

	data, _ := os.ReadFile(cfgPath(home))
	if contains(string(data), "secret") {
		t.Error("plaintext token found in cli.json — it should be encrypted")
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestDelete_removesFile(t *testing.T) {
	home := tempHome(t)
	writeRaw(t, home, `{"apiUrl":"https://api.futrou.com"}`)

	if err := Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(cfgPath(home)); !os.IsNotExist(err) {
		t.Error("config file still exists after Delete")
	}
}

func TestDelete_noFile(t *testing.T) {
	tempHome(t)
	if err := Delete(); err != nil {
		t.Errorf("Delete with no file should not error: %v", err)
	}
}

// ── DeriveKey / key isolation ────────────────────────────────────────────────
//
// These tests verify that changing any single component of the fingerprint
// (CLI name, machine ID, or user ID) produces a different key, so a token
// encrypted with one identity cannot be decrypted with another.

func TestDeriveKey_differentCliId(t *testing.T) {
	k1 := utils.DeriveKey("futrou-cli", "machine-abc", "1000")
	k2 := utils.DeriveKey("other-app", "machine-abc", "1000")
	if string(k1) == string(k2) {
		t.Error("different CLI names produced the same key")
	}
}

func TestDeriveKey_differentMachineId(t *testing.T) {
	k1 := utils.DeriveKey("futrou-cli", "machine-abc", "1000")
	k2 := utils.DeriveKey("futrou-cli", "machine-xyz", "1000")
	if string(k1) == string(k2) {
		t.Error("different machine IDs produced the same key")
	}
}

func TestDeriveKey_differentUserId(t *testing.T) {
	k1 := utils.DeriveKey("futrou-cli", "machine-abc", "1000")
	k2 := utils.DeriveKey("futrou-cli", "machine-abc", "1001")
	if string(k1) == string(k2) {
		t.Error("different user IDs produced the same key")
	}
}

func TestDeriveKey_sameFingerprintSameKey(t *testing.T) {
	k1 := utils.DeriveKey("futrou-cli", "machine-abc", "1000")
	k2 := utils.DeriveKey("futrou-cli", "machine-abc", "1000")
	if string(k1) != string(k2) {
		t.Error("same fingerprint inputs produced different keys")
	}
}

func TestEncryptDecrypt_wrongMachineId(t *testing.T) {
	keyA := utils.DeriveKey("futrou-cli", "machine-A", "1000")
	keyB := utils.DeriveKey("futrou-cli", "machine-B", "1000")

	enc := utils.EncryptWithKey("token-secret", keyA)
	_, err := utils.DecryptWithKey(enc, keyB)
	if err == nil {
		t.Error("expected decryption to fail with a different machine ID key")
	}
}

func TestEncryptDecrypt_wrongUserId(t *testing.T) {
	keyA := utils.DeriveKey("futrou-cli", "machine-abc", "1000")
	keyB := utils.DeriveKey("futrou-cli", "machine-abc", "1001")

	enc := utils.EncryptWithKey("token-secret", keyA)
	_, err := utils.DecryptWithKey(enc, keyB)
	if err == nil {
		t.Error("expected decryption to fail with a different user ID key")
	}
}

func TestEncryptDecrypt_wrongCliId(t *testing.T) {
	keyA := utils.DeriveKey("futrou-cli", "machine-abc", "1000")
	keyB := utils.DeriveKey("other-app", "machine-abc", "1000")

	enc := utils.EncryptWithKey("token-secret", keyA)
	_, err := utils.DecryptWithKey(enc, keyB)
	if err == nil {
		t.Error("expected decryption to fail with a different CLI name key")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
