package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"lsm/internal/prompt"
	"lsm/internal/runner"
	"lsm/internal/state"
)

var (
	cfgFile string
	dryRun  bool
	yes     bool
)

var rootCmd = &cobra.Command{
	Use:   "lsm",
	Short: "Linux Server Manager — base setup + ops",
	Long: `lsm provisions and manages a Debian server: firewall, SSH hardening,
Docker rootless, fail2ban, unattended-upgrades, timesync, sysctl, hostname.
Run without args for the interactive menu.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		runner.DryRun = dryRun
		return nil
	},
	// No subcommand → interactive menu.
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMenu()
	},
	SilenceUsage:  true,
	SilenceErrors: false,
}

// SetVersion is called from main with the version injected at build time.
func SetVersion(v string) { rootCmd.Version = v }

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/lsm/config.yaml", "config file path")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "log actions without executing")
	rootCmd.PersistentFlags().BoolVarP(&yes, "yes", "y", false, "auto-confirm prompts")
}

// RequireAdmin gates destructive operations. Admin = real root, where the
// invoking user is also root: $SUDO_USER unset OR $SUDO_USER == "root".
// `sudo lsm <destructive>` from a non-root user (e.g. dev24) is rejected —
// those flows must come from a root shell (console / IPMI / `su -` with
// root password / direct SSH as root). Operator-class commands (validate,
// system update, system reboot, status, port list, overview) bypass this
// and only need RequireRoot.
func RequireAdmin() error {
	if err := runner.RequireRoot(); err != nil {
		return err
	}
	su := os.Getenv("SUDO_USER")
	if su == "" || su == "root" {
		return nil
	}
	return fmt.Errorf("this operation requires direct root login (not via sudo). SUDO_USER=%s. Use the console / IPMI or `su -` with the root password", su)
}

// markInstalled records a successful module run in state.yaml.
// Best-effort: load failures are non-fatal (state is observability, not a critical path).
func markInstalled(name string) {
	st, err := state.Load(cfgFile)
	if err != nil {
		return
	}
	if st.MarkInstalled(name) {
		_ = st.Save()
	}
}

// shouldOpen evaluates the auto_open_ports policy.
// Uses prompt.Confirm so we share the single bufio.Reader on stdin —
// fmt.Scanln would race with the buffered reader and lose input.
func shouldOpen(desc, policy string) bool {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "true", "yes", "sim":
		return true
	case "false", "no", "nao", "não":
		runner.Log("auto_open_ports=false → not opening %s", desc)
		return false
	default:
		if yes {
			return true
		}
		return prompt.Confirm(fmt.Sprintf("Open %s in UFW?", desc), true)
	}
}
