package sysupdate

import (
	"os"
	"strings"

	"lsm/internal/runner"
)

// Update refreshes the apt package index.
func Update() error {
	return aptGet("update", "-qq")
}

// Upgrade installs available upgrades for installed packages without
// removing or installing new ones (uses dist-upgrade only if needed).
func Upgrade() error {
	return aptGet("upgrade", "-y", "-qq",
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold")
}

// Autoremove removes packages that are no longer required.
func Autoremove() error {
	return aptGet("autoremove", "-y", "-qq")
}

// AutoClean cleans the local package cache.
func AutoClean() error {
	return aptGet("autoclean", "-y", "-qq")
}

// RestartPending uses needrestart (if available) to restart services that
// loaded outdated libraries during the upgrade. Returns true if it ran.
// Avoids prompting; reboots are never triggered automatically.
func RestartPending() (bool, error) {
	if !runner.HasCommand("needrestart") {
		return false, nil
	}
	// -r a: automatically restart services
	// -q  : quiet
	// -m a: mode auto (no interactive prompts)
	return true, runner.Run("needrestart", "-r", "a", "-m", "a", "-q")
}

// EnsureNeedrestart installs needrestart if missing (best-effort).
func EnsureNeedrestart() error {
	if runner.HasCommand("needrestart") {
		return nil
	}
	return aptGet("install", "-y", "-qq", "needrestart")
}

// RebootNow triggers an immediate reboot via systemd.
func RebootNow() error {
	return runner.Run("systemctl", "reboot")
}

// ScheduleRebootAt schedules a reboot for the next occurrence of the given
// systemd OnCalendar spec (e.g. "*-*-* 04:00:00" = next 4 AM). Idempotent:
// if a previous lsm-reboot timer exists, it's replaced.
func ScheduleRebootAt(calendarSpec string) error {
	// Stop any previous scheduled reboot from lsm.
	_ = runner.Run("systemctl", "stop", "lsm-reboot.timer")
	_ = runner.Run("systemctl", "stop", "lsm-reboot.service")
	return runner.Run("systemd-run",
		"--on-calendar="+calendarSpec,
		"--unit=lsm-reboot.service",
		"/sbin/shutdown", "-r", "now", "lsm scheduled reboot")
}

// CancelScheduledReboot removes any pending lsm-scheduled reboot.
func CancelScheduledReboot() error {
	_ = runner.Run("systemctl", "stop", "lsm-reboot.timer")
	return runner.Run("systemctl", "stop", "lsm-reboot.service")
}

// RebootRequired reports whether /var/run/reboot-required exists,
// optionally returning the list of packages that triggered it.
func RebootRequired() (bool, []string) {
	if _, err := os.Stat("/var/run/reboot-required"); err != nil {
		return false, nil
	}
	var pkgs []string
	if data, err := os.ReadFile("/var/run/reboot-required.pkgs"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				pkgs = append(pkgs, line)
			}
		}
	}
	return true, pkgs
}

func aptGet(args ...string) error {
	full := append([]string{"apt-get"}, args...)
	return runner.RunEnv(map[string]string{
		"DEBIAN_FRONTEND":              "noninteractive",
		"NEEDRESTART_MODE":             "a",
		"NEEDRESTART_SUSPEND":          "1",
		"APT_LISTCHANGES_FRONTEND":     "none",
	}, full[0], full[1:]...)
}
