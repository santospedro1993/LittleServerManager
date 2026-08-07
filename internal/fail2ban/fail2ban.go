package fail2ban

import (
	"fmt"
	"os"
	"strings"

	"erp24/internal/runner"
)

func Installed() bool { return runner.HasCommand("fail2ban-server") }

func Install() error {
	runner.Log("Garantir fail2ban instalado...")
	if err := runner.AptGet("update", "-qq"); err != nil {
		return err
	}
	return runner.AptGet("install", "-y", "-qq", "fail2ban")
}

// WriteJailConfig writes /etc/fail2ban/jail.local with sshd jail tuned to sshPort.
// ignoreIPs are appended to the default ignoreip list.
func WriteJailConfig(sshPort int, ignoreIPs []string) error {
	ignore := []string{"127.0.0.1/8", "::1"}
	ignore = append(ignore, ignoreIPs...)

	body := fmt.Sprintf(`# Managed by erp24 — local overrides; do not edit by hand.
[DEFAULT]
ignoreip = %s
bantime  = 1h
findtime = 10m
maxretry = 5
backend  = systemd

[sshd]
enabled = true
port    = %d
`, strings.Join(ignore, " "), sshPort)

	return os.WriteFile("/etc/fail2ban/jail.local", []byte(body), 0644)
}

func Enable() error {
	if err := runner.Run("systemctl", "enable", "fail2ban"); err != nil {
		return err
	}
	return runner.Run("systemctl", "restart", "fail2ban")
}

// Status returns "fail2ban-client status sshd" output (jail summary).
func Status() (string, error) {
	return runner.Capture("fail2ban-client", "status", "sshd")
}

// JailLoaded reports whether the named jail appears in
// `fail2ban-client status`'s Jail list. A jail that's enabled in
// jail.local but failed to start (bad filter regex, missing logpath
// when backend != systemd, etc.) does NOT show up here — so this is
// the right signal to distinguish "daemon up" from "actually guarding".
//
// Returns false + error when fail2ban-client itself fails (daemon
// down, socket missing). False + nil error means the daemon answered
// but the jail isn't loaded.
func JailLoaded(name string) (bool, error) {
	out, err := runner.Capture("fail2ban-client", "status")
	if err != nil {
		return false, fmt.Errorf("fail2ban-client status: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "Jail list:") {
			continue
		}
		idx := strings.Index(line, ":")
		for _, j := range strings.Split(line[idx+1:], ",") {
			if strings.TrimSpace(j) == name {
				return true, nil
			}
		}
	}
	return false, nil
}

// JailHealthy runs `fail2ban-client status <name>` and reports whether
// it returned without error. JailLoaded checks the jail is listed;
// JailHealthy confirms the filter actually started and can be queried
// — a jail can be listed but in a faulted state.
func JailHealthy(name string) bool {
	_, err := runner.Capture("fail2ban-client", "status", name)
	return err == nil
}
