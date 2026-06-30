package config

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// machineID returns a stable identifier for the current device.
// It is not secret — any local process can read it — but it ties an
// encrypted token to the device it was created on: copying the config
// file to another machine yields a different key and decryption fails.
// Returns "" if no stable identifier could be found.
func machineID() string {
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
