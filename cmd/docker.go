package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/docker"
	"lsm/internal/runner"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Install Docker + setup rootless user",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		runner.Section("Docker: remover pacotes conflituosos")
		if err := docker.RemoveConflicts(); err != nil {
			return err
		}

		runner.Section("Docker: instalar repositório oficial")
		if err := docker.InstallRepo(); err != nil {
			return err
		}

		runner.Section("Docker: instalar engine")
		if err := docker.InstallEngine(); err != nil {
			return err
		}
		docker.DisableRootful()

		runner.Section(fmt.Sprintf("Docker rootless: user '%s'", cfg.Docker.RootlessUser))
		return docker.SetupRootlessUser(cfg.Docker.RootlessUser)
	},
}

func init() { rootCmd.AddCommand(dockerCmd) }
