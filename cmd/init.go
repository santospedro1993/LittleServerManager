package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/prompt"
)

func currentHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Setup wizard: prompts for values and writes config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWizard()
	},
}

func init() { rootCmd.AddCommand(initCmd) }

// runWizard prompts the user for intent values and writes the config file.
// Whitelist of IPs is NOT persisted to config — it's derived from UFW state
// (use `lsm add-ip` after setup). Password for the SSH user is NOT persisted
// either — it's prompted only when the user is actually created.
func runWizard() error {
	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║  Setup Inicial — Wizard                    ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Printf("Config file: %s\n\n", cfgFile)

	if config.Exists(cfgFile) {
		if !prompt.Confirm("Config já existe. Sobrescrever?", false) {
			fmt.Println("Cancelado.")
			return nil
		}
	}

	c := &config.Config{}

	fmt.Println("─── Timezone ─────────────────────────────────")
	fmt.Println("Fuso horário do servidor. Afeta logs e ficheiros.")
	fmt.Println("Ex: Europe/Lisbon, Europe/London, UTC.")
	c.Timezone = prompt.Ask("Timezone", "Europe/Lisbon")

	fmt.Println()
	fmt.Println("─── Hostname ─────────────────────────────────")
	fmt.Println("Nome curto da máquina (aparece na shell e em logs).")
	fmt.Println("Vazio = não tocar no hostname atual.")
	c.Hostname = prompt.Ask("Hostname (vazio salta)", currentHostname())
	if c.Hostname != "" {
		fmt.Println()
		fmt.Println("FQDN = nome completo com domínio (ex: srv01.exemplo.com).")
		fmt.Println("Vazio se não tens domínio configurado.")
		c.FQDN = prompt.Ask("FQDN (vazio para nenhum)", "")
	}

	fmt.Println()
	fmt.Println("─── SSH ──────────────────────────────────────")
	fmt.Println("User não-root para administrar via SSH (com sudo).")
	fmt.Println("Substitui login direto como root (que será bloqueado).")
	c.SSH.User = prompt.Ask("Username", "dev24")
	fmt.Println()
	fmt.Println("Porta SSH alternativa (não-22). Reduz brute-force automatizado.")
	fmt.Println("Tem de estar livre. 2210 é seguro como default.")
	c.SSH.Port = prompt.AskInt("Porta SSH", 2210)

	fmt.Println()
	fmt.Println("─── Docker ───────────────────────────────────")
	fmt.Println("User dedicado para correr containers em modo rootless.")
	fmt.Println("Containers correm como este user, não como root.")
	c.Docker.RootlessUser = prompt.Ask("Docker rootless user", "docker24")

	fmt.Println()
	fmt.Println("─── Política de portas em UFW ────────────────")
	fmt.Println("Quando módulos precisam de abrir uma porta na firewall:")
	fmt.Println("  true  → abre sem perguntar (rápido)")
	fmt.Println("  false → nunca abre (geres tu manualmente)")
	fmt.Println("  ask   → pergunta caso a caso (mais seguro, recomendado)")
	idx := prompt.Choose("Escolhe política", []string{
		"ask   — perguntar caso a caso (recomendado)",
		"true  — abrir automaticamente",
		"false — não abrir nunca",
	})
	c.Network.AutoOpenPorts = []string{"ask", "true", "false"}[idx-1]

	c.SetPath(cfgFile)
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0755); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Printf("\nConfig escrita em %s\n", cfgFile)
	fmt.Println()
	fmt.Println("Notas:")
	fmt.Println("  • Password do user SSH é pedida no momento da criação (não fica em ficheiro).")
	fmt.Println("  • Whitelist de IPs gere-se com 'lsm add-ip <IP>' / 'lsm remove-ip <IP>'.")
	fmt.Println("    A firewall (UFW) é a fonte da verdade.")
	return nil
}
