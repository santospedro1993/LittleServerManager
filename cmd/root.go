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
Docker engine, fail2ban, unattended-upgrades, timesync, sysctl, hostname.
Run without args for the interactive menu.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		runner.DryRun = dryRun
		// Best-effort: ensure /usr/local/bin/lsm wrapper exists so that
		// non-root users can type plain `lsm` without /usr/sbin in PATH.
		// Errors are non-fatal (we may not even be running as root yet).
		if os.Geteuid() == 0 {
			ensureShellWrapper()
		}
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

// ensureShellWrapper drops /usr/local/bin/lsm — a tiny shell wrapper that
// re-execs the real binary via sudo. Reasoning: the real binary lives in
// /usr/sbin/lsm (Debian convention for root-only tools), but `/usr/sbin`
// is NOT in a non-root user's default PATH. Without this wrapper, a user
// like dev24 typing plain `lsm` after a reboot gets "command not found".
//
// The wrapper is in /usr/local/bin which IS in every user's default PATH.
// It calls `sudo /usr/sbin/lsm "$@"` — the sudoers drop-in (see
// internal/ssh.GrantPasswordlessLSM) makes that pass without a prompt for
// the configured ssh user. For root invocations it's a no-op (sudo
// from root doesn't prompt either way).
//
// Best-effort + idempotent: writes only if the file is missing or content
// drifted; errors are swallowed (logged at debug only).
func ensureShellWrapper() {
	const path = "/usr/local/bin/lsm"
	want := "#!/bin/sh\n# Managed by lsm — resolves `lsm` from non-root PATHs.\nexec sudo /usr/sbin/lsm \"$@\"\n"
	if cur, err := os.ReadFile(path); err == nil && string(cur) == want {
		return
	}
	_ = os.WriteFile(path, []byte(want), 0755)
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
