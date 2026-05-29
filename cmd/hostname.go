package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"erp24/internal/config"
	"erp24/internal/hostname"
	"erp24/internal/runner"
)

var hostnameCmd = &cobra.Command{
	Use:   "hostname",
	Short: "Set system hostname + update /etc/hosts (uses config.hostname/fqdn)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		if cfg.Hostname == "" {
			return fmt.Errorf("config: hostname vazio — define em %s", cfgFile)
		}
		runner.Section(fmt.Sprintf("hostname: %s (fqdn=%q)", cfg.Hostname, cfg.FQDN))
		if err := hostname.Apply(cfg.Hostname, cfg.FQDN); err != nil {
			return err
		}
		runner.Log("Hostname agora: %s", cfg.Hostname)
		markInstalled("hostname")
		return nil
	},
}

func init() { rootCmd.AddCommand(hostnameCmd) }
