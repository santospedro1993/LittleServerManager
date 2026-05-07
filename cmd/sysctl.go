package cmd

import (
	"github.com/spf13/cobra"

	"lsm/internal/runner"
	"lsm/internal/sysctl"
)

var sysctlCmd = &cobra.Command{
	Use:   "sysctl",
	Short: "Apply kernel hardening + tuning (writes /etc/sysctl.d/99-lsm.conf)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		runner.Section("sysctl: write " + sysctl.ConfPath + " + apply")
		if err := sysctl.Write(); err != nil {
			return err
		}
		if err := sysctl.Apply(); err != nil {
			return err
		}
		runner.Log("sysctl aplicado.")
		return nil
	},
}

func init() { rootCmd.AddCommand(sysctlCmd) }
