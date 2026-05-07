package fail2ban

import (
	"fmt"
	"os"
	"strings"

	"lsm/internal/runner"
)

func Installed() bool { return runner.HasCommand("fail2ban-server") }

func Install() error {
	runner.Log("Garantir fail2ban instalado...")
	if err := runner.Run("apt-get", "update", "-qq"); err != nil {
		return err
	}
	return runner.Run("apt-get", "install", "-y", "-qq", "fail2ban")
}

// WriteJailConfig writes /etc/fail2ban/jail.local with sshd jail tuned to sshPort.
// ignoreIPs are appended to the default ignoreip list.
func WriteJailConfig(sshPort int, ignoreIPs []string) error {
	ignore := []string{"127.0.0.1/8", "::1"}
	ignore = append(ignore, ignoreIPs...)

	body := fmt.Sprintf(`# Managed by lsm — local overrides; do not edit by hand.
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
