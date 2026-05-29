package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"erp24/internal/prompt"
	"erp24/internal/runner"
	"erp24/internal/sysupdate"
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
	Short:  "Alias for `erp24 system update`",
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

	runner.Section("system update: configure needrestart (list-only, no inline restarts)")
	if err := sysupdate.WriteNeedrestartConfig(); err != nil {
		runner.Log("warning: failed to write needrestart drop-in (%v) — apt upgrade may show interactive dialog if needrestart is installed.", err)
	}

	runner.Section("system update: apt update")
	if err := sysupdate.Update(); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}

	runner.Section("system update: ensure needrestart present")
	if err := sysupdate.EnsureNeedrestart(); err != nil {
		runner.Log("warning: needrestart install failed (%v) — service-restart detection limited to /var/run/reboot-required.", err)
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

	// Detection only — never restart services inline. Restarting things like
	// dbus or systemd-logind during a provisioning run can drop the sshd
	// session we're running through. Whole-server reboot is the safe answer.
	runner.Section("system update: detect services using outdated libs")
	pending, _ := sysupdate.PendingRestarts()
	rebootNeeded, rebootPkgs := sysupdate.RebootRequired()

	if !pending.HasAny() && !rebootNeeded {
		runner.Log("No restarts pending.")
		return nil
	}

	fmt.Println()
	fmt.Println("⚠ Restart conditions detected:")
	if rebootNeeded {
		fmt.Println("  • /var/run/reboot-required is set (kernel/libc upgrade).")
		if len(rebootPkgs) > 0 {
			fmt.Println("    Packages requesting reboot:")
			for _, p := range rebootPkgs {
				fmt.Printf("      - %s\n", p)
			}
		}
	}
	if pending.KernelStale {
		fmt.Println("  • Running kernel differs from the installed one.")
	}
	if pending.MicrocodeStale {
		fmt.Println("  • CPU microcode update pending.")
	}
	if len(pending.Services) > 0 {
		fmt.Println("  • Services running with outdated libraries:")
		for _, s := range pending.Services {
			fmt.Printf("      - %s\n", s)
		}
	}
	fmt.Println()
	fmt.Println("erp24 doesn't restart services individually (risk of dropping the")
	fmt.Println("active sshd session). A full reboot resolves all of the above.")
	return promptReboot()
}

func promptReboot() error {
	if yes {
		runner.Log("--yes active: NOT rebooting. Run `sudo erp24 system reboot` when ready.")
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
		runner.Log("Reboot scheduled for next 04:00 (systemd-run timer 'erp24-reboot').")
		runner.Log("To cancel: sudo systemctl stop erp24-reboot.timer erp24-reboot.service")
	case 3:
		runner.Log("OK — reboot deferred. Reminder: `sudo erp24 system reboot` when ready.")
	}
	return nil
}
