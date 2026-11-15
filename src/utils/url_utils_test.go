package utils

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeApiUrlAcceptsEverySpelling(t *testing.T) {
	for in, want := range map[string]string{
		"":                              "",
		"api.futrou.com":                "https://api.futrou.com",
		"api.futrou.com/":               "https://api.futrou.com",
		"api.futrou.com/v2":             "https://api.futrou.com",
		"https://api.futrou.com":        "https://api.futrou.com",
		"https://api.futrou.com/v2/":    "https://api.futrou.com",
		"HTTPS://api.futrou.com":        "HTTPS://api.futrou.com",
		"api.futrou.com:8443":           "https://api.futrou.com:8443",
		"10.0.0.5:8585":                 "https://10.0.0.5:8585",
		"localhost:4000":                "http://localhost:4000",
		"127.0.0.1:4000":                "http://127.0.0.1:4000",
		"[::1]:4000":                    "http://[::1]:4000",
		"http://localhost:4000/v2":      "http://localhost:4000",
		"wss://api.futrou.com":          "https://api.futrou.com",
		"ws://localhost:4000":           "http://localhost:4000",
		"  https://api.futrou.com/v2  ": "https://api.futrou.com",
		"[2001:db8::1]:8443":            "https://[2001:db8::1]:8443",
		"https://example.test/v2/":      "https://example.test",
	} {
		if got := NormalizeApiUrl(in); got != want {
			t.Errorf("NormalizeApiUrl(%q) = %q, want %q", in, got, want)
		}
	}
}

// Normalizing twice changes nothing, so a URL that was already normalized (read
// from the stored config) is found under the same key as one typed by the user.
func TestNormalizeApiUrlIsIdempotent(t *testing.T) {
	for _, in := range []string{"api.futrou.com", "localhost:4000", "wss://x.test/v2/", "[::1]:4000", "https://a.test/v2"} {
		once := NormalizeApiUrl(in)
		if twice := NormalizeApiUrl(once); twice != once {
			t.Errorf("NormalizeApiUrl(%q) = %q, then %q", in, once, twice)
		}
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "LOCALHOST": true, "app.localhost": true, "127.0.0.1": true, "::1": true,
		"api.futrou.com": false, "10.0.0.5": false, "": false, "notlocalhost": false,
	} {
		if got := isLoopbackHost(host); got != want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

// There is one NormalizeApiUrl and one UUID pattern: a second copy drifts (as
// the config package's did) and then a token is stored under one spelling of a
// URL and looked up under another.
func TestNoSecondCopyOfTheSharedHelpers(t *testing.T) {
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		isUtils := strings.HasPrefix(filepath.ToSlash(path), "../utils/")
		if !isUtils && strings.Contains(text, "func NormalizeApiUrl(") {
			t.Errorf("%s defines NormalizeApiUrl: use utils.NormalizeApiUrl", path)
		}
		if !strings.HasSuffix(filepath.ToSlash(path), "utils/uuid_utils.go") && strings.Contains(text, "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}") {
			t.Errorf("%s has its own UUID pattern: use utils.IsUuid", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
