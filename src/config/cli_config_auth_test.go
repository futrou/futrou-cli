package config

import (
	"errors"
	"testing"
)

func TestErrCIRequiresToken_message(t *testing.T) {
	if !errors.Is(ErrCIRequiresToken, ErrCIRequiresToken) {
		t.Fatal("ErrCIRequiresToken should be comparable via errors.Is")
	}
	got := ErrCIRequiresToken.Error()
	want := "not logged in — CI environment requires a non-empty --api-token (or FUTROU_API_TOKEN)"
	if got != want {
		t.Fatalf("ErrCIRequiresToken.Error() = %q, want %q", got, want)
	}
}
