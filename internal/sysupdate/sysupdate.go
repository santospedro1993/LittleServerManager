package sysupdate

import (
	"os"
	"strings"

	"erp24/internal/runner"
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

// EnsureNeedrestart installs needrestart if missing AND drops a config
// snippet that flips it into list-only mode. We use needrestart for
// detection — never to restart services. The whole-server reboot prompt
// driven from PendingRestarts() is the only escalation we offer.
func EnsureNeedrestart() error {
	if !runner.HasCommand("needrestart") {
		if err := aptGet("install", "-y", "-qq", "needrestart"); err != nil {
			return err
		}
	}
	return WriteNeedrestartConfig()
}

// WriteNeedrestartConfig drops /etc/needrestart/conf.d/99-erp24.conf so the
// apt-integrated needrestart hook never tries to restart services and never
// shows the interactive "Which services should be restarted?" dialog.
// We do detection ourselves after the upgrade and ask the user about a full
// reboot — restarting individual daemons inline is too risky for a
// provisioning tool (e.g. dbus.service restart can drop sshd sessions).
func WriteNeedrestartConfig() error {
	const path = "/etc/needrestart/conf.d/99-erp24.conf"
	body := `# Managed by erp24 — list-only mode.
# erp24 reads pending restarts via 'needrestart -b -p' and prompts for a full
# reboot; we never let the apt hook restart services inline.
$nrconf{restart} = 'l';
$nrconf{kernelhints} = -1;
$nrconf{ucodehints} = 0;
`
	// Best-effort: directory may not exist if needrestart was just queued for
	// install in the same dpkg run. The config drop-in is read at next
	// invocation.
	_ = os.MkdirAll("/etc/needrestart/conf.d", 0755)
	return os.WriteFile(path, []byte(body), 0644)
}

// PendingRestart summarises what needrestart reports after an upgrade.
type PendingRestart struct {
	Services       []string // user-visible systemd units that loaded old libs
	KernelStale    bool     // running kernel != on-disk kernel
	MicrocodeStale bool     // CPU microcode update pending
}

// HasAny reports whether any reboot-worthy condition is present.
func (p PendingRestart) HasAny() bool {
	return len(p.Services) > 0 || p.KernelStale || p.MicrocodeStale
}

// PendingRestarts parses `needrestart -b -p` (batch + brief, machine-readable)
// and returns the structured result. Returns zero value if needrestart is
// missing — callers should treat that as "unknown, don't recommend reboot".
func PendingRestarts() (PendingRestart, error) {
	var p PendingRestart
	if !runner.HasCommand("needrestart") {
		return p, nil
	}
	out, err := runner.Capture("needrestart", "-b", "-p")
	// needrestart exits non-zero (1 = pending, 2 = kernel pending, 3 = both)
	// to signal status — that's not a real error for us. We still parse out.
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "NEEDRESTART-SVC:"):
			svc := strings.TrimSpace(strings.TrimPrefix(line, "NEEDRESTART-SVC:"))
			if svc != "" {
				p.Services = append(p.Services, svc)
			}
		case strings.HasPrefix(line, "NEEDRESTART-KSTA:"):
			// 1=ok, 2=ABI compat new kernel, 3=version mismatch
			val := strings.TrimSpace(strings.TrimPrefix(line, "NEEDRESTART-KSTA:"))
			if val != "" && val != "1" {
				p.KernelStale = true
			}
		case strings.HasPrefix(line, "NEEDRESTART-UCSTA:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "NEEDRESTART-UCSTA:"))
			if val != "" && val != "1" {
				p.MicrocodeStale = true
			}
		}
	}
	if err != nil && len(out) == 0 {
		return p, err
	}
	return p, nil
}

// RebootNow triggers an immediate reboot via systemd.
func RebootNow() error {
	return runner.Run("systemctl", "reboot")
}

// ScheduleRebootAt schedules a reboot for the next occurrence of the given
// systemd OnCalendar spec (e.g. "*-*-* 04:00:00" = next 4 AM). Idempotent:
// if a previous erp24-reboot timer exists, it's replaced.
func ScheduleRebootAt(calendarSpec string) error {
	// Stop any previous scheduled reboot from erp24.
	_ = runner.Run("systemctl", "stop", "erp24-reboot.timer")
	_ = runner.Run("systemctl", "stop", "erp24-reboot.service")
	return runner.Run("systemd-run",
		"--on-calendar="+calendarSpec,
		"--unit=erp24-reboot.service",
		"/sbin/shutdown", "-r", "now", "erp24 scheduled reboot")
}

// CancelScheduledReboot removes any pending erp24-scheduled reboot.
func CancelScheduledReboot() error {
	_ = runner.Run("systemctl", "stop", "erp24-reboot.timer")
	return runner.Run("systemctl", "stop", "erp24-reboot.service")
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
		"DEBIAN_FRONTEND": "noninteractive",
		// NEEDRESTART_MODE=l + NEEDRESTART_SUSPEND=1 keep the apt hook from
		// ever showing the "Which services should be restarted?" dialog.
		// The drop-in at /etc/needrestart/conf.d/99-erp24.conf is the durable
		// guarantee; these env vars are belt-and-braces for the first run
		// before the drop-in lands.
		"NEEDRESTART_MODE":         "l",
		"NEEDRESTART_SUSPEND":      "1",
		"APT_LISTCHANGES_FRONTEND": "none",
	}, full[0], full[1:]...)
}
