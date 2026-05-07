package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/docker"
	"lsm/internal/prompt"
	"lsm/internal/runner"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Install Docker + setup rootless user",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		cfg, err := config.Load(cfgFile)
		if err != nil {
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
		docker.DisableRootful()

		runner.Section(fmt.Sprintf("Docker rootless: user '%s'", cfg.Docker.RootlessUser))

		// Password is requested only when the user doesn't yet exist —
		// re-runs against an already-provisioned user keep whatever
		// password is in place. Same pattern as the ssh module.
		var password string
		if !docker.UserExists(cfg.Docker.RootlessUser) {
			password = prompt.AskPassword(fmt.Sprintf("Password for new user '%s'", cfg.Docker.RootlessUser))
		}
		if err := docker.SetupRootlessUser(cfg.Docker.RootlessUser, password); err != nil {
			return err
		}
		markInstalled("docker")
		return nil
	},
}

func init() { rootCmd.AddCommand(dockerCmd) }
