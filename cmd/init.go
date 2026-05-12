package cmd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/prompt"
)

// posixUserRe matches valid Linux usernames per useradd(8) NAME_REGEX:
// lowercase letter or underscore, then up to 31 of [a-z0-9_-]. We don't
// allow uppercase to dodge case-folding surprises across LDAP/Samba/etc.
var posixUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// reservedUsers blocks names that ship with Debian or are conventional
// system accounts. Creating/repurposing any of these via lsm risks
// clobbering passwords or breaking the host (e.g. systemd-* users).
var reservedUsers = map[string]bool{
	"root": true, "daemon": true, "bin": true, "sys": true, "sync": true,
	"games": true, "man": true, "lp": true, "mail": true, "news": true,
	"uucp": true, "proxy": true, "www-data": true, "backup": true,
	"list": true, "irc": true, "gnats": true, "nobody": true, "_apt": true,
	"systemd-network": true, "systemd-resolve": true, "systemd-timesync": true,
	"messagebus": true, "sshd": true, "polkitd": true,
}

// askValidNewUser prompts for a username, looping until the input is a
// valid POSIX name AND not on the reserved list. If the user already
// exists on the system, asks for explicit confirmation — re-running the
// wizard against an existing-but-lsm-managed user is legitimate, but
// pointing it at an unrelated account would mutate sudo membership and
// (in CreateUser) skip password setup silently.
func askValidNewUser(question, def string) string {
	for {
		name := prompt.Ask(question, def)
		switch {
		case !posixUserRe.MatchString(name):
			fmt.Println("Invalid username: use [a-z_][a-z0-9_-]{0,31} (start with letter/underscore, ≤32 chars).")
			continue
		case reservedUsers[name]:
			fmt.Printf("'%s' is a reserved system user — pick another.\n", name)
			continue
		}
		if _, err := user.Lookup(name); err == nil {
			fmt.Printf("User '%s' already exists on this host.\n", name)
			fmt.Println("  Continuing keeps its current password and removes it from the 'sudo' group.")
			if !prompt.Confirm("Use this existing user anyway?", false) {
				continue
			}
		}
		return name
	}
}

func currentHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Setup wizard: prompts for values and writes config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		return runWizard()
	},
}

func init() { rootCmd.AddCommand(initCmd) }

// runWizard prompts the user for intent values and writes the config file.
// Whitelist of IPs is NOT persisted to config — it's derived from UFW state
// (use `lsm port allow` / `lsm add-ip` after setup). Password for the SSH
// user is NOT persisted either — it's prompted when the user is created.
func runWizard() error {
	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║  Initial Setup — Wizard                    ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Printf("Config file: %s\n\n", cfgFile)

	if config.Exists(cfgFile) {
		if !prompt.Confirm("Config already exists. Overwrite?", false) {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	c := &config.Config{SchemaVersion: config.CurrentSchemaVersion}

	fmt.Println("─── Timezone ─────────────────────────────────")
	fmt.Println("Server timezone. Affects logs and timestamps.")
	fmt.Println("e.g. Etc/UTC, Europe/Lisbon, Europe/London.")
	c.Timezone = prompt.Ask("Timezone", "Etc/UTC")

	fmt.Println()
	fmt.Println("─── Hostname ─────────────────────────────────")
	fmt.Println("Short machine name (shown in shell prompts and logs).")
	fmt.Println("Empty = leave current hostname untouched.")
	c.Hostname = prompt.Ask("Hostname (empty skips)", currentHostname())
	if c.Hostname != "" {
		fmt.Println()
		fmt.Println("FQDN = full name with domain (e.g. srv01.example.com).")
		fmt.Println("Empty if you don't have a domain configured.")
		c.FQDN = prompt.Ask("FQDN (empty for none)", "")
	}

	fmt.Println()
	fmt.Println("─── SSH ──────────────────────────────────────")
	fmt.Println("Non-root admin user for SSH. Replaces direct root login")
	fmt.Println("(which will be disabled).")
	c.SSH.User = askValidNewUser("Username", "dev24")
	fmt.Println()
	fmt.Println("SSH port (non-22 reduces automated brute-force attempts).")
	fmt.Println("Must be free on the host. 2210 is a safe default.")
	c.SSH.Port = prompt.AskInt("SSH port", 2210)

	fmt.Println()
	fmt.Println("─── Docker ───────────────────────────────────")
	fmt.Println("Dedicated user that runs containers in rootless mode.")
	fmt.Println("Containers run as this user, not root.")
	c.Docker.RootlessUser = askValidNewUser("Docker rootless user", "docker24")

	fmt.Println()
	fmt.Println("─── UFW open-port policy ─────────────────────")
	fmt.Println("When a module needs to open a firewall port:")
	fmt.Println("  ask   → prompt each time (safer, recommended)")
	fmt.Println("  true  → open automatically")
	fmt.Println("  false → never open (you manage UFW manually)")
	idx := prompt.Choose("Pick a policy", []string{
		"ask   — prompt each time (recommended)",
		"true  — open automatically",
		"false — never open",
	})
	c.Network.AutoOpenPorts = []string{"ask", "true", "false"}[idx-1]

	fmt.Println()
	fmt.Println("─── Modules ──────────────────────────────────")
	fmt.Println("Baseline modules (firewall, ssh, sysctl, timesync, hostname,")
	fmt.Println("fail2ban, upgrades) are always installed — they are the")
	fmt.Println("security baseline. Only opt-in modules are prompted below.")

	// Mandatory baseline.
	c.Modules.Firewall = true
	c.Modules.SSH = true
	c.Modules.Sysctl = true
	c.Modules.Timesync = true
	c.Modules.Hostname = c.Hostname != "" // skip if no hostname configured
	c.Modules.Fail2ban = true
	c.Modules.Upgrades = true

	// Opt-in modules (ask one by one — extend this block when adding new ones).
	c.Modules.Docker = prompt.Confirm("Install docker (engine + rootless user)?", true)

	c.SetPath(cfgFile)
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0755); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Printf("\nConfig written to %s\n", cfgFile)
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  • SSH password is requested at user creation time (not stored on disk).")
	fmt.Println("  • IP whitelist is managed via `lsm port allow` (UFW is the source of truth).")
	return nil
}
