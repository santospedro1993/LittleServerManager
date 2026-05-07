package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"lsm/internal/prompt"
	"lsm/internal/runner"
	"lsm/internal/sysupdate"
)

var updateCmd = &cobra.Command{
	Use:   "update-server",
	Short: "apt update + upgrade + autoremove (sem reboot pendente de serviços)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdateServer()
	},
}

func init() { rootCmd.AddCommand(updateCmd) }

func runUpdateServer() error {
	if err := runner.RequireRoot(); err != nil {
		return err
	}

	runner.Section("update-server: garantir needrestart (auto-restart de serviços)")
	if err := sysupdate.EnsureNeedrestart(); err != nil {
		runner.Log("Aviso: needrestart não instalado (%v) — serviços podem ficar a usar libs antigas até reboot.", err)
	}

	runner.Section("update-server: apt update")
	if err := sysupdate.Update(); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}

	runner.Section("update-server: apt upgrade")
	if err := sysupdate.Upgrade(); err != nil {
		return fmt.Errorf("apt upgrade: %w", err)
	}

	runner.Section("update-server: apt autoremove")
	if err := sysupdate.Autoremove(); err != nil {
		return fmt.Errorf("apt autoremove: %w", err)
	}

	runner.Section("update-server: apt autoclean")
	if err := sysupdate.AutoClean(); err != nil {
		runner.Log("autoclean falhou (não crítico): %v", err)
	}

	runner.Section("update-server: restart de serviços com libs antigas")
	if ran, err := sysupdate.RestartPending(); err != nil {
		runner.Log("needrestart falhou: %v", err)
	} else if !ran {
		runner.Log("needrestart não disponível — salta auto-restart de serviços.")
	} else {
		runner.Log("Serviços com libs antigas reiniciados.")
	}

	if need, pkgs := sysupdate.RebootRequired(); need {
		fmt.Println()
		fmt.Println("⚠ KERNEL/libc atualizados — reboot do servidor recomendado.")
		if len(pkgs) > 0 {
			fmt.Println("  Pacotes que pediram reboot:")
			for _, p := range pkgs {
				fmt.Printf("   • %s\n", p)
			}
		}
		return promptReboot()
	}
	runner.Log("Sem reboot pendente.")
	return nil
}

func promptReboot() error {
	if yes {
		runner.Log("--yes ativo: NÃO reinicio automaticamente. Corre 'sudo reboot' quando puderes.")
		return nil
	}
	idx := prompt.Choose("Como queres tratar o reboot?", []string{
		"Reboot agora",
		"Agendar para amanhã às 04:00",
		"Não reiniciar (eu faço manualmente)",
	})
	switch idx {
	case 1:
		runner.Log("A reiniciar agora...")
		return sysupdate.RebootNow()
	case 2:
		if err := sysupdate.ScheduleRebootAt("*-*-* 04:00:00"); err != nil {
			return fmt.Errorf("agendar reboot: %w", err)
		}
		runner.Log("Reboot agendado p/ próximo 04:00 (systemd-run timer 'lsm-reboot').")
		runner.Log("Para cancelar: sudo systemctl stop lsm-reboot.timer lsm-reboot.service")
	case 3:
		runner.Log("OK — reboot adiado. Lembra-te: 'sudo reboot' quando puderes.")
	}
	return nil
}
