package commands

import "testing"

func TestUpgradeDisabledByEnv(t *testing.T) {
	ts := newTestServer(t)
	for _, v := range []string{"1", "true", "yes"} {
		t.Setenv("FUTROU_UPGRADE_DISABLED", v)
		_, err := runArgs(t, ts, "upgrade")
		if err == nil || err.Error() != errUpgradeViaNpm.Error() {
			t.Fatalf("value %q: expected npm upgrade error, got %v", v, err)
		}
	}
}

func TestUpgradeDisabledFalseValues(t *testing.T) {
	for _, v := range []string{"", "0", "false", "off", " No "} {
		t.Setenv("FUTROU_UPGRADE_DISABLED", v)
		if upgradeDisabled() {
			t.Errorf("value %q should not disable upgrade", v)
		}
	}
}
