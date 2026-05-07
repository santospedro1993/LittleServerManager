package ufw

import (
	"fmt"
	"strings"

	"lsm/internal/runner"
)

func Installed() bool { return runner.HasCommand("ufw") }

func Install() error {
	if Installed() {
		runner.Log("UFW já presente.")
		return nil
	}
	runner.Log("UFW não instalado — a instalar...")
	if err := runner.Run("apt-get", "update", "-qq"); err != nil {
		return err
	}
	return runner.Run("apt-get", "install", "-y", "-qq", "ufw")
}

func SetDefaults() error {
	if err := runner.Run("ufw", "default", "deny", "incoming"); err != nil {
		return err
	}
	return runner.Run("ufw", "default", "allow", "outgoing")
}

func Status() (string, error) { return runner.Capture("ufw", "status") }

func IsActive() bool {
	s, err := Status()
	if err != nil {
		return false
	}
	return strings.Contains(s, "Status: active")
}

func Enable() error {
	if IsActive() {
		runner.Log("UFW já ativo.")
		return nil
	}
	return runner.Run("ufw", "--force", "enable")
}

// PortPermitted returns true if "PORT/proto" is already allowed.
func PortPermitted(port int, proto string) bool {
	s, err := Status()
	if err != nil {
		return false
	}
	needle := fmt.Sprintf("%d/%s", port, proto)
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, needle) && strings.Contains(t, "ALLOW") {
			return true
		}
	}
	return false
}

// Allow opens a port from any source.
func Allow(port int, proto, comment string) error {
	return runner.Run("ufw", "allow",
		fmt.Sprintf("%d/%s", port, proto),
		"comment", comment)
}

// AllowFrom opens a port from a specific source IP/CIDR.
func AllowFrom(ip string, port int, proto, comment string) error {
	return runner.Run("ufw", "allow",
		"from", ip,
		"to", "any",
		"port", fmt.Sprintf("%d", port),
		"proto", proto,
		"comment", comment)
}

// DeleteAllowFrom removes a specific "allow from IP to any port PORT proto PROTO" rule.
func DeleteAllowFrom(ip string, port int, proto string) error {
	return runner.Run("ufw", "delete", "allow",
		"from", ip,
		"to", "any",
		"port", fmt.Sprintf("%d", port),
		"proto", proto)
}

// DeleteAllow removes a "allow PORT/proto" rule (any source).
func DeleteAllow(port int, proto string) error {
	return runner.Run("ufw", "delete", "allow", fmt.Sprintf("%d/%s", port, proto))
}

// OpenWhitelisted opens port for each IP in allowedIPs, or to all if empty.
func OpenWhitelisted(port int, proto, label string, allowedIPs []string) error {
	if len(allowedIPs) == 0 {
		if err := Allow(port, proto, label); err != nil {
			return err
		}
		runner.Log("UFW: %d/%s aberta a TODOS (%s).", port, proto, label)
		return nil
	}
	for _, ip := range allowedIPs {
		if err := AllowFrom(ip, port, proto, fmt.Sprintf("%s - %s", label, ip)); err != nil {
			return err
		}
	}
	runner.Log("UFW: %d/%s permitida para %d IP(s) (%s).", port, proto, len(allowedIPs), label)
	return nil
}
