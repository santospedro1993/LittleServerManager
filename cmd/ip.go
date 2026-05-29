package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"erp24/internal/prompt"
	"erp24/internal/runner"
	"erp24/internal/state"
	"erp24/internal/ufw"
)

var addIPCmd = &cobra.Command{
	Use:   "add-ip [ip]",
	Short: "Allow IP/CIDR for all managed ports in UFW",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
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
		if err := RequireAdmin(); err != nil {
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
// ports per UFW. Source of truth: the firewall itself, not config. Reads from
// both INPUT and FORWARD chains depending on each port's Kind.
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
		for _, src := range kindSpecificSources(p.EffectiveKind(), p.Port, p.Proto) {
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
		return fmt.Errorf("UFW não instalado — corre 'erp24 firewall' primeiro")
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
		kind := p.EffectiveKind()
		already := false
		for _, src := range kindSpecificSources(kind, p.Port, p.Proto) {
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
		if err := kindAllowFrom(kind, ip, p.Port, p.Proto, label); err != nil {
			return fmt.Errorf("ufw allow %s %d/%s: %w", ip, p.Port, p.Proto, err)
		}
		if kindIsOpenToAll(kind, p.Port, p.Proto) {
			_ = kindDeleteAllow(kind, p.Port, p.Proto)
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

	closing := portsLosingAllRules(st, ip)
	sshClosing := false
	for _, p := range closing {
		if isSSHPort(p.Port, p.Proto) {
			sshClosing = true
			break
		}
	}
	if sshClosing && !confirmCloseSSH(ip) {
		return nil
	}

	for _, p := range st.ManagedPorts {
		kind := p.EffectiveKind()
		if err := kindDeleteAllowFrom(kind, ip, p.Port, p.Proto); err != nil {
			runner.Log("UFW: delete %d/%s from %s failed (ignored): %v", p.Port, p.Proto, ip, err)
		}
		if len(kindSpecificSources(kind, p.Port, p.Proto)) == 0 && !kindIsOpenToAll(kind, p.Port, p.Proto) {
			runner.Log("UFW: %d/%s sem regras — porta fechada.", p.Port, p.Proto)
		}
	}
	fmt.Printf("Removido %s. Whitelist UFW = %v\n", ip, currentWhitelist())
	return nil
}
