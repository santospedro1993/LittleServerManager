package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"erp24/internal/config"
	"erp24/internal/runner"
	"erp24/internal/timesync"
)

var timesyncCmd = &cobra.Command{
	Use:   "timesync",
	Short: "Configure systemd-timesyncd + timezone",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		runner.Section(fmt.Sprintf("timesync: timezone=%s + enable systemd-timesyncd", cfg.Timezone))
		if err := timesync.Install(); err != nil {
			return err
		}
		if err := timesync.SetTimezone(cfg.Timezone); err != nil {
			return err
		}
		if err := timesync.Enable(); err != nil {
			return err
		}
		runner.Log("NTP ativo. Timezone: %s. Sincronizado: %v.",
			timesync.CurrentTimezone(), timesync.Synced())
		markInstalled("timesync")
		return nil
	},
}

var timesyncSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Force immediate NTP re-sync (restart systemd-timesyncd)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		if err := timesync.ForceSync(); err != nil {
			return err
		}
		out, _ := timesync.Status()
		fmt.Println(out)
		return nil
	},
}

var timesyncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current time, timezone and sync state",
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := timesync.Status()
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	},
}

func init() {
	timesyncCmd.AddCommand(timesyncSyncCmd)
	timesyncCmd.AddCommand(timesyncStatusCmd)
	rootCmd.AddCommand(timesyncCmd)
}
