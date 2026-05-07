package ssh

import (
	"fmt"
	"os/user"

	"lsm/internal/runner"
	"lsm/internal/ufw"
)

func userExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

func CreateUser(name, password string) error {
	if userExists(name) {
		runner.Log("User '%s' já existe.", name)
	} else {
		if err := runner.Run("useradd", "-m", "-s", "/bin/bash", name); err != nil {
			return err
		}
		if err := runner.Stdin(fmt.Sprintf("%s:%s", name, password), "chpasswd"); err != nil {
			return err
		}
		runner.Log("User '%s' criado.", name)
	}
	return runner.Run("usermod", "-aG", "sudo", name)
}

func Harden(port int) error {
	cfg := "/etc/ssh/sshd_config"
	repls := [][2]string{
		{`^#\?Port .*`, fmt.Sprintf("Port %d", port)},
		{`^#\?PermitRootLogin .*`, "PermitRootLogin no"},
		{`^#\?PasswordAuthentication .*`, "PasswordAuthentication yes"},
	}
	for _, r := range repls {
		expr := fmt.Sprintf("s/%s/%s/", r[0], r[1])
		if err := runner.Run("sed", "-i", expr, cfg); err != nil {
			return err
		}
	}
	if err := runner.Run("systemctl", "restart", "sshd"); err != nil {
		return err
	}
	runner.Log("sshd na porta %d, root login desativado.", port)
	return nil
}

// OpenFirewall opens SSH_PORT in UFW respecting whitelist.
func OpenFirewall(port int, allowedIPs []string) error {
	return ufw.OpenWhitelisted(port, "tcp", "SSH", allowedIPs)
}
