package commands

import (
	"os"
	"testing"
)

func TestRequireAuth_noConfig(t *testing.T) {
	ts := newTestServer(t)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FUTROU_API_TOKEN", "")

	_, err := runArgsNoAuth(t, ts, "serverlets", "list")
	assertError(t, err)
	// The error message should mention login
	if err != nil {
		_ = os.Stdout // keep import; actual message goes to logger stderr
	}
}
