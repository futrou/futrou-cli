package utils

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestDeriveKeyDependsOnEveryComponent(t *testing.T) {
	base := DeriveKey("cli", "machine", "1000")
	if len(base) != 32 {
		t.Fatalf("an AES-256 key is 32 bytes, got %d", len(base))
	}
	if !bytes.Equal(base, DeriveKey("cli", "machine", "1000")) {
		t.Fatal("the key must be deterministic")
	}
	for name, other := range map[string][]byte{
		"cli":     DeriveKey("other", "machine", "1000"),
		"machine": DeriveKey("cli", "other", "1000"),
		"user":    DeriveKey("cli", "machine", "1001"),
	} {
		if bytes.Equal(base, other) {
			t.Errorf("changing the %s must change the key", name)
		}
	}
}

func TestEncryptDecryptWithKey(t *testing.T) {
	key := DeriveKey("cli", "machine", "1000")
	sealed := EncryptWithKey("secret-token", key)
	if !strings.HasPrefix(sealed, EncPrefix) || strings.Contains(sealed, "secret-token") {
		t.Fatalf("the token must be encrypted: %s", sealed)
	}
	if again := EncryptWithKey("secret-token", key); again == sealed {
		t.Fatal("every encryption needs a fresh nonce")
	}
	got, err := DecryptWithKey(sealed, key)
	if err != nil || got != "secret-token" {
		t.Fatalf("round trip = %q, %v", got, err)
	}
	if _, err := DecryptWithKey(sealed, DeriveKey("cli", "other-machine", "1000")); err == nil || !strings.Contains(err.Error(), "different device") {
		t.Fatalf("another device must not decrypt: %v", err)
	}
	if got, err := DecryptWithKey("legacy-plaintext", key); err != nil || got != "legacy-plaintext" {
		t.Fatalf("values without the prefix are legacy plaintext: %q, %v", got, err)
	}
	if _, err := DecryptWithKey(EncPrefix+"%%%not-base64", key); err == nil {
		t.Fatal("invalid base64 must fail")
	}
	if _, err := DecryptWithKey(EncPrefix+"AAAA", key); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("a payload shorter than the nonce is malformed: %v", err)
	}
	if got := EncryptWithKey("x", []byte("short")); got != "x" {
		t.Fatalf("an unusable key must not produce a fake ciphertext: %q", got)
	}
	if _, err := DecryptWithKey(sealed, []byte("short")); err == nil {
		t.Fatal("an invalid key size must fail")
	}
}

func TestTamperedCiphertextIsRejected(t *testing.T) {
	key := DeriveKey("cli", "machine", "1000")
	stored := EncryptWithKey("secret", key)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, EncPrefix))
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	sealed := EncPrefix + base64.StdEncoding.EncodeToString(raw)
	if _, err := DecryptWithKey(sealed, key); err == nil {
		t.Fatal("GCM must detect a modified ciphertext")
	}
}

func TestEncryptAndDecryptTokenUseTheDeviceKey(t *testing.T) {
	if got := EncryptToken("", "futrou"); got != "" {
		t.Fatalf("an empty token stays empty: %q", got)
	}
	sealed := EncryptToken("tok", "futrou")
	if !strings.HasPrefix(sealed, EncPrefix) {
		t.Fatalf("sealed = %q", sealed)
	}
	if got, err := DecryptToken(sealed, "futrou"); err != nil || got != "tok" {
		t.Fatalf("round trip = %q, %v", got, err)
	}
	if _, err := DecryptToken(sealed, "another-cli"); err == nil {
		t.Fatal("a different CLI name derives a different key")
	}
	if got, err := DecryptToken("", "futrou"); err != nil || got != "" {
		t.Fatalf("empty = %q, %v", got, err)
	}
	if got, err := DecryptToken("plain", "futrou"); err != nil || got != "plain" {
		t.Fatalf("legacy plaintext = %q, %v", got, err)
	}
	_ = MachineID() // may be empty in containers; it must never panic
}

func TestStripPrefix(t *testing.T) {
	if rest, ok := stripPrefix("enc:v1:abc", "enc:v1:"); !ok || rest != "abc" {
		t.Fatalf("rest = %q, %v", rest, ok)
	}
	for _, in := range []string{"", "enc", "xnc:v1:abc"} {
		if _, ok := stripPrefix(in, "enc:v1:"); ok {
			t.Errorf("%q must not match", in)
		}
	}
}
