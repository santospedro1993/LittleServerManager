package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"lsm/internal/prompt"
	"lsm/internal/runner"
	"lsm/internal/sysupdate"
)

var systemCmd = &cobra.Command{
	Use:   "system",
	Short: "System maintenance (update, reboot)",
}

var systemUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "apt update + upgrade + autoremove (auto-restart services, prompt reboot)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSystemUpdate()
	},
}

var systemRebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot the server now or schedule it",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		return promptReboot()
	},
}

// Backward-compat alias for the previous top-level name.
var updateServerCmd = &cobra.Command{
	Use:    "update-server",
	Short:  "Alias for `lsm system update`",
	Hidden: true,
	RunE:   systemUpdateCmd.RunE,
}

func init() {
	systemCmd.AddCommand(systemUpdateCmd, systemRebootCmd)
	rootCmd.AddCommand(systemCmd, updateServerCmd)
}

func runSystemUpdate() error {
	if err := runner.RequireRoot(); err != nil {
		return err
	}

	runner.Section("system update: ensure needrestart")
	if err := sysupdate.EnsureNeedrestart(); err != nil {
		runner.Log("warning: needrestart not installed (%v) — services may keep outdated libs until reboot.", err)
	}

	runner.Section("system update: apt update")
	if err := sysupdate.Update(); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}

	runner.Section("system update: apt upgrade")
	if err := sysupdate.Upgrade(); err != nil {
		return fmt.Errorf("apt upgrade: %w", err)
	}

	runner.Section("system update: apt autoremove")
	if err := sysupdate.Autoremove(); err != nil {
		return fmt.Errorf("apt autoremove: %w", err)
	}

	runner.Section("system update: apt autoclean")
	if err := sysupdate.AutoClean(); err != nil {
		runner.Log("autoclean failed (non-critical): %v", err)
	}

	runner.Section("system update: restart services with outdated libs")
	if ran, err := sysupdate.RestartPending(); err != nil {
		runner.Log("needrestart failed: %v", err)
	} else if !ran {
		runner.Log("needrestart not available — skipping auto-restart.")
	} else {
		runner.Log("Services with outdated libs restarted.")
	}

	if need, pkgs := sysupdate.RebootRequired(); need {
		fmt.Println()
		fmt.Println("⚠ Kernel/libc upgraded — server reboot recommended.")
		if len(pkgs) > 0 {
			fmt.Println("  Packages requesting reboot:")
			for _, p := range pkgs {
				fmt.Printf("   • %s\n", p)
			}
		}
		return promptReboot()
	}
	runner.Log("No reboot pending.")
	return nil
}

func promptReboot() error {
	if yes {
		runner.Log("--yes active: NOT rebooting. Run `sudo lsm system reboot` when ready.")
		return nil
	}
	idx := prompt.Choose("How do you want to handle the reboot?", []string{
		"Reboot now",
		"Schedule for next 04:00",
		"Don't reboot (I'll do it manually)",
	})
	switch idx {
	case 1:
		runner.Log("Rebooting now...")
		return sysupdate.RebootNow()
	case 2:
		if err := sysupdate.ScheduleRebootAt("*-*-* 04:00:00"); err != nil {
			return fmt.Errorf("schedule reboot: %w", err)
		}
		runner.Log("Reboot scheduled for next 04:00 (systemd-run timer 'lsm-reboot').")
		runner.Log("To cancel: sudo systemctl stop lsm-reboot.timer lsm-reboot.service")
	case 3:
		runner.Log("OK — reboot deferred. Reminder: `sudo lsm system reboot` when ready.")
	}
	return nil
}
