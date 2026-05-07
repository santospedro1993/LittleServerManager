package cmd

import (
	"github.com/spf13/cobra"

	"lsm/internal/runner"
	"lsm/internal/upgrades"
)

var upgradesCmd = &cobra.Command{
	Use:   "upgrades",
	Short: "Install + enable unattended-upgrades (auto security patches)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
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
		return nil
	},
}

func init() { rootCmd.AddCommand(upgradesCmd) }
