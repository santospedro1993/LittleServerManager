package timesync

import (
	"strings"

	"erp24/internal/runner"
)

// Install is a no-op: systemd-timesyncd ships with systemd.
func Install() error {
	runner.Log("systemd-timesyncd já vem com systemd; sem instalação.")
	return nil
}

// Enable activates NTP via timedatectl + ensures the service is running.
func Enable() error {
	if err := runner.Run("timedatectl", "set-ntp", "true"); err != nil {
		return err
	}
	return runner.Run("systemctl", "enable", "--now", "systemd-timesyncd")
}

func SetTimezone(tz string) error {
	if tz == "" {
		return nil
	}
	return runner.Run("timedatectl", "set-timezone", tz)
}

// ForceSync restarts timesyncd to trigger an immediate sync attempt.
func ForceSync() error {
	runner.Log("A forçar re-sync (restart systemd-timesyncd)...")
	return runner.Run("systemctl", "restart", "systemd-timesyncd")
}

func Synced() bool {
	out, _ := runner.Capture("timedatectl", "show", "-p", "NTPSynchronized", "--value")
	return strings.TrimSpace(out) == "yes"
}

func NTPEnabled() bool {
	out, _ := runner.Capture("timedatectl", "show", "-p", "NTP", "--value")
	return strings.TrimSpace(out) == "yes"
}

func CurrentTimezone() string {
	out, _ := runner.Capture("timedatectl", "show", "-p", "Timezone", "--value")
	return strings.TrimSpace(out)
}

func Status() (string, error) { return runner.Capture("timedatectl", "status") }
