package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

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
func shouldOpen(desc, policy string) bool {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "true", "yes", "sim":
		return true
	case "false", "no", "nao", "não":
		runner.Log("auto_open_ports=false → NÃO abrir %s", desc)
		return false
	default:
		if yes {
			return true
		}
		fmt.Printf("Abrir %s em UFW? [Y/n]: ", desc)
		var ans string
		fmt.Scanln(&ans)
		ans = strings.ToLower(strings.TrimSpace(ans))
		return ans != "n" && ans != "no" && ans != "nao"
	}
}
