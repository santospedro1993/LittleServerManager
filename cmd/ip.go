package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"lsm/internal/prompt"
	"lsm/internal/runner"
	"lsm/internal/state"
	"lsm/internal/ufw"
)

var addIPCmd = &cobra.Command{
	Use:   "add-ip [ip]",
	Short: "Allow IP/CIDR for all managed ports in UFW",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		var ip string
		if len(args) == 1 {
			ip = args[0]
		} else {
			ip = prompt.AskIPOrCIDR("IP/CIDR a permitir")
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
	Short: "Revoke IP/CIDR from managed ports in UFW",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runner.RequireRoot(); err != nil {
			return err
		}
		var ip string
		if len(args) == 1 {
			ip = args[0]
		} else {
			cur := currentWhitelist()
			if len(cur) == 0 {
				fmt.Println("Nenhum IP específico ativo nas portas geridas.")
				return nil
			}
			fmt.Println("\nIPs atuais (UFW):")
			for i, e := range cur {
				fmt.Printf("  %d) %s\n", i+1, e)
			}
			idx := prompt.Choose("Qual remover?", cur)
			ip = cur[idx-1]
		}
		return removeIP(ip)
	},
}

func init() {
	rootCmd.AddCommand(addIPCmd)
	rootCmd.AddCommand(removeIPCmd)
}

// currentWhitelist returns the union of specific IPs/CIDRs allowed on managed
// ports per UFW. Source of truth: the firewall itself, not config.
func currentWhitelist() []string {
	if !ufw.Installed() {
		return nil
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range st.ManagedPorts {
		for _, src := range ufw.SpecificSources(p.Port, p.Proto) {
			if !seen[src] {
				seen[src] = true
				out = append(out, src)
			}
		}
	}
	return out
}

func addIP(ip string) error {
	if !ufw.Installed() {
		return fmt.Errorf("UFW não instalado — corre 'lsm firewall' primeiro")
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	if len(st.ManagedPorts) == 0 {
		fmt.Println("Sem portas geridas em state.yaml — nada para abrir.")
		return nil
	}

	for _, p := range st.ManagedPorts {
		// Skip se já existe ALLOW from <ip>.
		already := false
		for _, src := range ufw.SpecificSources(p.Port, p.Proto) {
			if src == ip {
				already = true
				break
			}
		}
		if already {
			runner.Log("UFW: %d/%s já permite %s.", p.Port, p.Proto, ip)
			continue
		}
		label := fmt.Sprintf("%s - %s", p.Label, ip)
		if err := ufw.AllowFrom(ip, p.Port, p.Proto, label); err != nil {
			return fmt.Errorf("ufw allow %s %d/%s: %w", ip, p.Port, p.Proto, err)
		}
		// Se a porta tinha "Anywhere", remove-a — passou a estar restrita.
		if ufw.IsOpenToAll(p.Port, p.Proto) {
			_ = ufw.DeleteAllow(p.Port, p.Proto)
			runner.Log("UFW: %d/%s — fechado a todos, restrito a IPs específicos.", p.Port, p.Proto)
		}
	}
	fmt.Printf("Adicionado %s. Whitelist UFW = %v\n", ip, currentWhitelist())
	return nil
}

func removeIP(ip string) error {
	if !ufw.Installed() {
		return fmt.Errorf("UFW não instalado")
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}

	for _, p := range st.ManagedPorts {
		_ = ufw.DeleteAllowFrom(ip, p.Port, p.Proto)

		// Se a porta ficou sem IPs específicos E não estava aberta a todos,
		// reabre a todos para evitar locked-out.
		if len(ufw.SpecificSources(p.Port, p.Proto)) == 0 && !ufw.IsOpenToAll(p.Port, p.Proto) {
			if err := ufw.Allow(p.Port, p.Proto, p.Label); err != nil {
				return err
			}
			runner.Log("UFW: %d/%s sem IPs específicos — reaberto a todos.", p.Port, p.Proto)
		}
	}
	fmt.Printf("Removido %s. Whitelist UFW = %v\n", ip, currentWhitelist())
	return nil
}
