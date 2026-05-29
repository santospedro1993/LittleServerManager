package ssh

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"erp24/internal/runner"
	"erp24/internal/ufw"
)

func UserExists(name string) bool {
	_, err := user.Lookup(name)
	return err == nil
}

// CreateUser creates name with the given password (only if user doesn't exist).
// The user is intentionally NOT added to the sudo group — full root access via
// sudo is reserved for explicit root login. Operator-class erp24 commands are
// granted via a narrow /etc/sudoers.d/erp24 drop-in (see GrantPasswordlessERP24).
//
// If the user was previously in the sudo group (from older erp24 versions or
// manual setup), this function removes the membership so the security model
// stays consistent. Password is consumed once and never persisted by erp24.
func CreateUser(name, password string) error {
	if UserExists(name) {
		runner.Log("User '%s' already exists — keeping current password.", name)
	} else {
		if password == "" {
			return fmt.Errorf("empty password creating user '%s'", name)
		}
		if err := runner.Run("useradd", "-m", "-s", "/bin/bash", name); err != nil {
			return fmt.Errorf("useradd '%s' failed: %w (no changes made — pick another name or fix the underlying issue and retry)", name, err)
		}
		if err := runner.Stdin(fmt.Sprintf("%s:%s", name, password), "chpasswd"); err != nil {
			// Rollback: useradd succeeded but the account has no password.
			// Leaving it would create a passwordless account — depending on
			// PAM config (nullok) that could be a login vector. Tear down
			// before surfacing the error.
			runner.Log("WARNING: chpasswd failed for '%s' — rolling back useradd to avoid a passwordless account.", name)
			if rbErr := runner.Run("userdel", "-r", name); rbErr != nil {
				return fmt.Errorf("chpasswd failed (%w) AND rollback userdel failed (%v) — MANUAL CLEANUP NEEDED: `userdel -r %s`", err, rbErr, name)
			}
			return fmt.Errorf("set password for '%s': %w (user rolled back; retry)", name, err)
		}
		runner.Log("User '%s' created.", name)
	}
	// Ensure user is NOT in the sudo group (operator-only model).
	// `gpasswd -d` errors if not a member — ignored via TryRun.
	runner.TryRun("gpasswd", "-d", name, "sudo")
	return nil
}

// Harden writes our settings to /etc/ssh/sshd_config.d/99-erp24.conf rather
// than editing the main sshd_config. Reasons:
//   - Debian 12+ ships an `Include /etc/ssh/sshd_config.d/*.conf` directive
//     and recommends drop-ins for local overrides.
//   - sed-replacing in the main file fails silently when a directive isn't
//     present (some distros omit the `#Port 22` comment line entirely).
//   - Drop-in is trivially reversible (rm the file) and survives package
//     upgrades that may rewrite sshd_config.
//
// Drop-in directives win because sshd takes the first occurrence of each
// option, and Include is processed early in Debian's default sshd_config.
func Harden(port int) error {
	const dropIn = "/etc/ssh/sshd_config.d/99-erp24.conf"
	body := fmt.Sprintf(`# Managed by erp24 — do not edit by hand.
Port %d
PermitRootLogin no
PasswordAuthentication yes
`, port)
	if err := os.WriteFile(dropIn, []byte(body), 0644); err != nil {
		return fmt.Errorf("write %s: %w", dropIn, err)
	}
	// Validate config before restart — broken sshd_config locks everyone out.
	if err := runner.Run("sshd", "-t"); err != nil {
		return fmt.Errorf("sshd config check failed (NOT restarting): %w", err)
	}
	// Debian aliases sshd → ssh; reload-or-restart is gentler than restart.
	if err := runner.Run("systemctl", "reload-or-restart", "ssh"); err != nil {
		// Fallback for systems where the unit is named sshd.
		if err2 := runner.Run("systemctl", "reload-or-restart", "sshd"); err2 != nil {
			return fmt.Errorf("restart ssh/sshd: %w", err)
		}
	}
	runner.Log("sshd on port %d, root login disabled (drop-in: %s).", port, dropIn)
	return nil
}

// GrantPasswordlessERP24 writes /etc/sudoers.d/erp24 so the given user can run
// `sudo erp24 ...` without a password prompt. The blanket NOPASSWD on the
// binary is fine: erp24 itself enforces the admin/operator split in-app via
// RequireAdmin (which rejects destructive ops when $SUDO_USER is set to a
// non-root user). Sudoers stays simple; the gate is one place to maintain.
//
// The file is validated with `visudo -cf` before being moved into place —
// a malformed drop-in would break sudo for everyone, so we never install
// it untested.
func GrantPasswordlessERP24(name string) error {
	const final = "/etc/sudoers.d/erp24"
	const tmp = "/etc/sudoers.d/.erp24.tmp"

	body := fmt.Sprintf(`# Managed by erp24 — do not edit by hand.
# Blanket NOPASSWD on the erp24 binary; in-app RequireAdmin blocks destructive
# subcommands when invoked by non-root, so the gate stays in one place.
%[1]s ALL=(root) NOPASSWD: /usr/sbin/erp24
`, name)

	if err := os.WriteFile(tmp, []byte(body), 0440); err != nil {
		return fmt.Errorf("write sudoers tmp: %w", err)
	}
	if err := runner.Run("visudo", "-cf", tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("sudoers syntax invalid (not installed): %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("move sudoers to %s: %w", final, err)
	}
	runner.Log("sudoers drop-in installed: '%s' may run `sudo erp24` without password (in-app RequireAdmin still gates destructive ops).", name)
	return nil
}

// SetAutoLaunchERP24 toggles a snippet in the user's login profile that runs
// `sudo erp24` on every interactive login. We pick `.profile` over
// `.bash_profile` because Debian's default user setup uses `.profile`
// (which sources `.bashrc`); creating a `.bash_profile` would shadow that
// and the user's normal shell setup would silently stop running.
//
// The snippet is bracketed by marker comments so we can remove it cleanly.
// Idempotent.
func SetAutoLaunchERP24(name string, enable bool) error {
	const beginMarker = "# >>> erp24 auto-launch >>>"
	const endMarker = "# <<< erp24 auto-launch <<<"

	u, err := user.Lookup(name)
	if err != nil {
		return fmt.Errorf("user %s: %w", name, err)
	}
	profile := u.HomeDir + "/.profile"

	existing, _ := os.ReadFile(profile)
	body := string(existing)

	// strip existing block (between markers, if present)
	if idx := strings.Index(body, beginMarker); idx >= 0 {
		end := strings.Index(body, endMarker)
		if end >= idx {
			body = body[:idx] + body[end+len(endMarker):]
		}
	}
	body = strings.TrimRight(body, "\n")

	if enable {
		snippet := "\n" + beginMarker + `
# Auto-run the erp24 menu on interactive login. The ERP24_LAUNCHED guard
# prevents a sudo-shell from re-triggering erp24 recursively. When the
# login is already root (su -, sudo -i, direct root) we skip sudo to
# avoid an unnecessary wrapper invocation.
if [ -t 0 ] && [ -z "$ERP24_LAUNCHED" ]; then
    export ERP24_LAUNCHED=1
    if [ "$(id -u)" = "0" ]; then
        /usr/sbin/erp24
    else
        sudo /usr/sbin/erp24
    fi
fi
` + endMarker + "\n"
		body += snippet
	} else if body != "" {
		body += "\n"
	}

	if err := os.WriteFile(profile, []byte(body), 0644); err != nil {
		return fmt.Errorf("write %s: %w", profile, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	_ = os.Chown(profile, uid, gid)
	if enable {
		runner.Log("Auto-launch enabled: `sudo erp24` runs on %s login (%s).", name, profile)
	} else {
		runner.Log("Auto-launch disabled for %s (%s).", name, profile)
	}
	return nil
}

// OpenFirewall opens the SSH port in UFW. Initial open is to all sources;
// per-IP whitelisting is applied later via `erp24 port allow` / `erp24 add-ip`
// (which read UFW state directly, not config). If the port already has
// specific ALLOW rules, this is a no-op so we don't widen access.
func OpenFirewall(port int) error {
	if len(ufw.SpecificSources(port, "tcp")) > 0 || ufw.IsOpenToAll(port, "tcp") {
		runner.Log("UFW: %d/tcp already has rules — kept as-is.", port)
		return nil
	}
	return ufw.Allow(port, "tcp", "SSH")
}
