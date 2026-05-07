package cmd

import (
	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/fail2ban"
	"lsm/internal/runner"
)

var fail2banCmd = &cobra.Command{
	Use:   "fail2ban",
	Short: "Install fail2ban + tune sshd jail using SSH port from config",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		runner.Section("fail2ban: install + jail.local + enable")
		if err := fail2ban.Install(); err != nil {
			return err
		}
		if err := fail2ban.WriteJailConfig(cfg.SSH.Port, cfg.Network.AllowedIPs); err != nil {
			return err
		}
		if err := fail2ban.Enable(); err != nil {
			return err
		}
		runner.Log("fail2ban a vigiar sshd na porta %d (bantime 1h, maxretry 5).", cfg.SSH.Port)
		return nil
	},
}

func init() { rootCmd.AddCommand(fail2banCmd) }
