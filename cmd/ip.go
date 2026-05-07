package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/prompt"
	"lsm/internal/runner"
	"lsm/internal/state"
	"lsm/internal/ufw"
)

var addIPCmd = &cobra.Command{
	Use:   "add-ip [ip]",
	Short: "Add IP/CIDR to whitelist + sync UFW for managed ports",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		var ip string
		if len(args) == 1 {
			ip = args[0]
		} else {
			ip = prompt.AskIPOrCIDR("IP/CIDR a adicionar")
			if ip == "" {
				fmt.Println("Cancelado.")
				return nil
			}
		}
		return addIP(ip)
	},
}

var removeIPCmd = &cobra.Command{
	Use:   "remove-ip [ip]",
	Short: "Remove IP/CIDR from whitelist + sync UFW",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		var ip string
		if len(args) == 1 {
			ip = args[0]
		} else {
			if len(cfg.Network.AllowedIPs) == 0 {
				fmt.Println("Lista vazia, nada a remover.")
				return nil
			}
			fmt.Println("\nIPs atuais:")
			for i, e := range cfg.Network.AllowedIPs {
				fmt.Printf("  %d) %s\n", i+1, e)
			}
			idx := prompt.Choose("Qual remover?", cfg.Network.AllowedIPs)
			ip = cfg.Network.AllowedIPs[idx-1]
		}
		return removeIP(ip)
	},
}

func init() {
	rootCmd.AddCommand(addIPCmd)
	rootCmd.AddCommand(removeIPCmd)
}

func addIP(ip string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}

	if !cfg.AddAllowedIP(ip) {
		fmt.Printf("%s já está na whitelist.\n", ip)
		return nil
	}

	// Aplicar nova regra para cada porta gerida.
	if !ufw.Installed() {
		fmt.Println("UFW não instalado — só atualizo config, não há regras a sincronizar.")
	} else {
		for _, p := range st.ManagedPorts {
			label := fmt.Sprintf("%s - %s", p.Label, ip)
			if err := ufw.AllowFrom(ip, p.Port, p.Proto, label); err != nil {
				return fmt.Errorf("ufw allow %s %d/%s: %w", ip, p.Port, p.Proto, err)
			}
		}

		// Se a lista passou de vazia para 1 IP, remove a regra "todos" das portas geridas.
		if len(cfg.Network.AllowedIPs) == 1 {
			for _, p := range st.ManagedPorts {
				_ = ufw.DeleteAllow(p.Port, p.Proto)
			}
		}
	}

	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("Adicionado %s. Whitelist = %v\n", ip, cfg.Network.AllowedIPs)
	return nil
}

func removeIP(ip string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}

	if !cfg.RemoveAllowedIP(ip) {
		fmt.Printf("%s não está na whitelist.\n", ip)
		return nil
	}

	if ufw.Installed() {
		for _, p := range st.ManagedPorts {
			_ = ufw.DeleteAllowFrom(ip, p.Port, p.Proto)
		}

		// Se ficou vazia, reabre as portas geridas a todos.
		if len(cfg.Network.AllowedIPs) == 0 {
			for _, p := range st.ManagedPorts {
				if err := ufw.Allow(p.Port, p.Proto, p.Label); err != nil {
					return err
				}
			}
		}
	}

	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("Removido %s. Whitelist = %v\n", ip, cfg.Network.AllowedIPs)
	return nil
}
