package cmd

import (
	"github.com/spf13/cobra"
)

var allCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all modules in order: firewall, timesync, sysctl, hostname, ssh, docker, fail2ban, upgrades",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAllModules()
	},
}

func init() { rootCmd.AddCommand(allCmd) }
