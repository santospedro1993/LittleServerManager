package cmd

import (
	"github.com/spf13/cobra"

	"erp24/internal/docker"
	"erp24/internal/runner"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Install Docker engine (rootful daemon)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}

		runner.Section("Docker: remove conflicting packages")
		if err := docker.RemoveConflicts(); err != nil {
			return err
		}

		runner.Section("Docker: install official repo")
		if err := docker.InstallRepo(); err != nil {
			return err
		}

		runner.Section("Docker: install engine")
		if err := docker.InstallEngine(); err != nil {
			return err
		}

		runner.Section("Docker: enable daemon")
		if err := docker.EnableDaemon(); err != nil {
			return err
		}

		markInstalled("docker")
		return nil
	},
}

func init() { rootCmd.AddCommand(dockerCmd) }
