package ssh

import (
	"fmt"
	"os/user"

	"lsm/internal/runner"
	"lsm/internal/ufw"
)

func UserExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

// CreateUser creates name with the given password (only if user doesn't exist),
// then ensures sudo membership. Password is consumed once and never persisted
// by lsm — caller is responsible for zeroing/discarding it after the call.
func CreateUser(name, password string) error {
	if UserExists(name) {
		runner.Log("User '%s' já existe — password mantida.", name)
	} else {
		if password == "" {
			return fmt.Errorf("password vazia ao criar user '%s'", name)
		}
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

// OpenFirewall opens the SSH port in UFW. Initial open is to all sources;
// per-IP whitelisting is applied later via `lsm add-ip` (which reads UFW state
// directly, not config). If the port already has specific ALLOW rules, this
// is a no-op so we don't widen access.
func OpenFirewall(port int) error {
	if len(ufw.SpecificSources(port, "tcp")) > 0 || ufw.IsOpenToAll(port, "tcp") {
		runner.Log("UFW: %d/tcp já tem regras — mantido.", port)
		return nil
	}
	return ufw.Allow(port, "tcp", "SSH")
}
