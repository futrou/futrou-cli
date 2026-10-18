package commands

import (
	"testing"
)

func TestLicense(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	out, err := runArgsNoAuth(t, ts, "license")
	assertNoError(t, err)
	assertContains(t, out, "MIT")
}

func TestLicense_json(t *testing.T) {
	ts := newTestServer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	out, err := runArgsNoAuth(t, ts, "--log-format", "json", "license")
	assertNoError(t, err)
	assertContains(t, out, "license")
	assertContains(t, out, "MIT")
}
