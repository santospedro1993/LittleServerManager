package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/prompt"
)

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

	c := &config.Config{}

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
	c.SSH.User = prompt.Ask("Username", "dev24")
	fmt.Println()
	fmt.Println("SSH port (non-22 reduces automated brute-force attempts).")
	fmt.Println("Must be free on the host. 2210 is a safe default.")
	c.SSH.Port = prompt.AskInt("SSH port", 2210)

	fmt.Println()
	fmt.Println("─── Docker ───────────────────────────────────")
	fmt.Println("Dedicated user that runs containers in rootless mode.")
	fmt.Println("Containers run as this user, not root.")
	c.Docker.RootlessUser = prompt.Ask("Docker rootless user", "docker24")

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
	fmt.Println("─── Modules to install ───────────────────────")
	fmt.Println("Pick which modules run after this wizard. SSH + firewall")
	fmt.Println("are mandatory (the security baseline depends on them).")

	c.Modules.Firewall = true
	c.Modules.SSH = true
	c.Modules.Sysctl = prompt.Confirm("Install module: sysctl  (kernel hardening)?", true)
	c.Modules.Timesync = prompt.Confirm("Install module: timesync (NTP + timezone)?", true)
	if c.Hostname != "" {
		c.Modules.Hostname = prompt.Confirm("Install module: hostname (apply chosen hostname)?", true)
	}
	c.Modules.Docker = prompt.Confirm("Install module: docker   (engine + rootless user)?", true)
	c.Modules.Fail2ban = prompt.Confirm("Install module: fail2ban (anti SSH brute-force)?", true)
	c.Modules.Upgrades = prompt.Confirm("Install module: upgrades (unattended security patches)?", true)

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
