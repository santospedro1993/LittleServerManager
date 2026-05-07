package ssh

import (
	"fmt"
	"os"
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

// GrantPasswordlessLSM writes /etc/sudoers.d/lsm so the given user can run
// /usr/sbin/lsm via sudo without being prompted for a password. The file is
// validated with `visudo -cf` before being moved into place — a malformed
// drop-in would break sudo for everyone, so we never install it untested.
func GrantPasswordlessLSM(name string) error {
	const final = "/etc/sudoers.d/lsm"
	const tmp = "/etc/sudoers.d/.lsm.tmp"

	body := fmt.Sprintf("# Managed by lsm — do not edit by hand.\n%s ALL=(root) NOPASSWD: /usr/sbin/lsm\n", name)

	if err := os.WriteFile(tmp, []byte(body), 0440); err != nil {
		return fmt.Errorf("escrever sudoers tmp: %w", err)
	}
	if err := runner.Run("visudo", "-cf", tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("sudoers syntax inválida (não foi instalado): %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("mover sudoers para %s: %w", final, err)
	}
	runner.Log("sudo NOPASSWD configurado: %s pode correr 'sudo lsm' sem password.", name)
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
