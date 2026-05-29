package cmd

import (
	"github.com/spf13/cobra"

	"erp24/internal/runner"
	"erp24/internal/upgrades"
)

var upgradesCmd = &cobra.Command{
	Use:   "upgrades",
	Short: "Install + enable unattended-upgrades (auto security patches)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		runner.Section("unattended-upgrades: install + enable periodic")
		if err := upgrades.Install(); err != nil {
			return err
		}
		if err := upgrades.EnablePeriodic(); err != nil {
			return err
		}
		runner.Log("unattended-upgrades ativo. Próximas updates de segurança aplicam-se automaticamente.")
		markInstalled("upgrades")
		return nil
	},
}

func init() { rootCmd.AddCommand(upgradesCmd) }
