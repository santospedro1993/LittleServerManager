package cmd

import (
	"github.com/spf13/cobra"

	"lsm/internal/runner"
	"lsm/internal/ufw"
)

var firewallCmd = &cobra.Command{
	Use:   "firewall",
	Short: "Bootstrap UFW (idempotent): install, defaults, port 22, enable",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		runner.Section("UFW: bootstrap idempotente")

		if err := ufw.Install(); err != nil {
			return err
		}
		if err := ufw.SetDefaults(); err != nil {
			return err
		}
		if ufw.PortPermitted(22, "tcp") {
			runner.Log("Porta 22 já permitida.")
		} else {
			if err := ufw.Allow(22, "tcp", "SSH bootstrap - remover após testar nova porta"); err != nil {
				return err
			}
			runner.Log("Porta 22 aberta (bootstrap).")
		}
		return ufw.Enable()
	},
}

func init() { rootCmd.AddCommand(firewallCmd) }
