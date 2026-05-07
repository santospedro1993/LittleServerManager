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

// runWizard prompts the user for values and writes the config file.
func runWizard() error {
	fmt.Println("=== Setup Inicial — config wizard ===")
	fmt.Printf("Config file: %s\n\n", cfgFile)

	if config.Exists(cfgFile) {
		if !prompt.Confirm("Config já existe. Sobrescrever?", false) {
			fmt.Println("Cancelado.")
			return nil
		}
	}

	c := &config.Config{}
	c.Timezone = prompt.Ask("Timezone", "Europe/Lisbon")
	c.Hostname = prompt.Ask("Hostname (vazio salta módulo)", currentHostname())
	if c.Hostname != "" {
		c.FQDN = prompt.Ask("FQDN (vazio para nenhum)", "")
	}
	c.SSH.User = prompt.Ask("SSH user", "dev24")
	c.SSH.Port = prompt.AskInt("SSH port", 2210)
	c.SSH.Password = prompt.Ask("SSH password (texto plano)", "ALTERA-ME-123")
	c.Docker.RootlessUser = prompt.Ask("Docker rootless user", "docker24")

	fmt.Println()
	fmt.Println("Whitelist de IPs (vazio → portas abrem a todos).")
	fmt.Println("Adiciona um por um, vazio para terminar.")
	for {
		ip := prompt.AskIPOrCIDR("IP/CIDR")
		if ip == "" {
			break
		}
		c.AddAllowedIP(ip)
		fmt.Printf("  + %s\n", ip)
	}

	idx := prompt.Choose("Política para abrir portas em UFW (auto_open_ports)", []string{
		"true  — módulos abrem automaticamente",
		"false — não abrir, gerir manualmente",
		"ask   — perguntar caso a caso (recomendado)",
	})
	c.Network.AutoOpenPorts = []string{"true", "false", "ask"}[idx-1]

	c.SetPath(cfgFile)
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0755); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Printf("\nConfig escrita em %s\n", cfgFile)
	return nil
}
