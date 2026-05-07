package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"lsm/internal/runner"
	"lsm/internal/state"
	"lsm/internal/ufw"
)

var portCmd = &cobra.Command{
	Use:   "port",
	Short: "Manage UFW rules for arbitrary ports (per-port whitelist)",
}

var portAddRestrict bool

var portAddCmd = &cobra.Command{
	Use:   "add <PORT>/<PROTO> [LABEL]",
	Short: "Register + open a port (default: open to all; --restrict opens nothing)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		port, proto, err := parsePortProto(args[0])
		if err != nil {
			return err
		}
		label := fmt.Sprintf("%d/%s", port, proto)
		if len(args) == 2 {
			label = args[1]
		}
		return portAdd(port, proto, label, portAddRestrict)
	},
}

var portRemoveCmd = &cobra.Command{
	Use:   "remove <PORT>/<PROTO>",
	Short: "Close port + unregister from lsm state",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		port, proto, err := parsePortProto(args[0])
		if err != nil {
			return err
		}
		return portRemove(port, proto)
	},
}

var portAllowCmd = &cobra.Command{
	Use:   "allow <PORT>/<PROTO> <IP>",
	Short: "Allow IP/CIDR for one port (drops Anywhere if first specific IP)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		port, proto, err := parsePortProto(args[0])
		if err != nil {
			return err
		}
		return portAllow(port, proto, args[1])
	},
}

var portRevokeCmd = &cobra.Command{
	Use:   "revoke <PORT>/<PROTO> <IP>",
	Short: "Revoke IP/CIDR for one port (re-opens to all if last specific IP)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RequireAdmin(); err != nil {
			return err
		}
		port, proto, err := parsePortProto(args[0])
		if err != nil {
			return err
		}
		return portRevoke(port, proto, args[1])
	},
}

var portListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed ports and their UFW sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		return portList()
	},
}

func init() {
	portAddCmd.Flags().BoolVar(&portAddRestrict, "restrict", false,
		"add port to state but DO NOT open in UFW — use 'port allow' to grant IPs")
	portCmd.AddCommand(portAddCmd, portRemoveCmd, portAllowCmd, portRevokeCmd, portListCmd)
	rootCmd.AddCommand(portCmd)
}

// parsePortProto turns "3306/tcp" into (3306, "tcp"). Default proto = "tcp".
func parsePortProto(spec string) (int, string, error) {
	parts := strings.SplitN(spec, "/", 2)
	port, err := strconv.Atoi(parts[0])
	if err != nil || port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("porta inválida: %q", parts[0])
	}
	proto := "tcp"
	if len(parts) == 2 {
		proto = strings.ToLower(parts[1])
		if proto != "tcp" && proto != "udp" {
			return 0, "", fmt.Errorf("protocolo inválido: %q (usa tcp ou udp)", parts[1])
		}
	}
	return port, proto, nil
}

func portAdd(port int, proto, label string, restrict bool) error {
	if !ufw.Installed() {
		return fmt.Errorf("UFW não instalado")
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	added := st.AddPort(state.ManagedPort{Port: port, Proto: proto, Label: label})
	if added {
		if err := st.Save(); err != nil {
			return err
		}
		runner.Log("Porta %d/%s registada como '%s'.", port, proto, label)
	} else {
		runner.Log("Porta %d/%s já estava registada.", port, proto)
	}
	if restrict {
		runner.Log("--restrict: NÃO abro nada. Usa 'lsm port allow %d/%s <IP>' para conceder acesso.", port, proto)
		return nil
	}
	if ufw.IsOpenToAll(port, proto) || len(ufw.SpecificSources(port, proto)) > 0 {
		runner.Log("UFW já tem regras para %d/%s — mantido.", port, proto)
		return nil
	}
	if err := ufw.Allow(port, proto, label); err != nil {
		return err
	}
	runner.Log("UFW: %d/%s aberta a TODOS.", port, proto)
	return nil
}

func portRemove(port int, proto string) error {
	if !ufw.Installed() {
		return fmt.Errorf("UFW não instalado")
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	// Apaga todas as regras desta porta no UFW (Anywhere + cada IP específico).
	if ufw.IsOpenToAll(port, proto) {
		_ = ufw.DeleteAllow(port, proto)
	}
	for _, ip := range ufw.SpecificSources(port, proto) {
		_ = ufw.DeleteAllowFrom(ip, port, proto)
	}
	if st.RemovePort(port, proto) {
		if err := st.Save(); err != nil {
			return err
		}
	}
	runner.Log("Porta %d/%s fechada e removida do state.", port, proto)
	return nil
}

func portAllow(port int, proto, ip string) error {
	if !ufw.Installed() {
		return fmt.Errorf("UFW não instalado")
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	if !st.HasPort(port, proto) {
		return fmt.Errorf("porta %d/%s não está em state — corre 'lsm port add %d/%s' primeiro", port, proto, port, proto)
	}
	for _, src := range ufw.SpecificSources(port, proto) {
		if src == ip {
			runner.Log("UFW: %d/%s já permite %s.", port, proto, ip)
			return nil
		}
	}
	label := fmt.Sprintf("%d/%s - %s", port, proto, ip)
	for _, p := range st.ManagedPorts {
		if p.Port == port && p.Proto == proto {
			label = fmt.Sprintf("%s - %s", p.Label, ip)
			break
		}
	}
	if err := ufw.AllowFrom(ip, port, proto, label); err != nil {
		return err
	}
	if ufw.IsOpenToAll(port, proto) {
		_ = ufw.DeleteAllow(port, proto)
		runner.Log("UFW: %d/%s — fechado a todos, restrito a IPs específicos.", port, proto)
	}
	runner.Log("UFW: %d/%s permite %s.", port, proto, ip)
	return nil
}

func portRevoke(port int, proto, ip string) error {
	if !ufw.Installed() {
		return fmt.Errorf("UFW não instalado")
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	_ = ufw.DeleteAllowFrom(ip, port, proto)
	if len(ufw.SpecificSources(port, proto)) == 0 && !ufw.IsOpenToAll(port, proto) {
		// reabre a todos para evitar locked-out involuntário
		label := fmt.Sprintf("%d/%s", port, proto)
		for _, p := range st.ManagedPorts {
			if p.Port == port && p.Proto == proto {
				label = p.Label
				break
			}
		}
		if err := ufw.Allow(port, proto, label); err != nil {
			return err
		}
		runner.Log("UFW: %d/%s sem IPs específicos — reaberto a todos.", port, proto)
	}
	runner.Log("UFW: %d/%s já não permite %s.", port, proto, ip)
	return nil
}

func portList() error {
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	if len(st.ManagedPorts) == 0 {
		fmt.Println("Sem portas geridas.")
		return nil
	}
	fmt.Println()
	fmt.Println("PORT/PROTO   LABEL                ESTADO UFW")
	fmt.Println("──────────────────────────────────────────────────────────")
	for _, p := range st.ManagedPorts {
		state := "[sem regra]"
		if ufw.Installed() {
			srcs := ufw.AllowedSources(p.Port, p.Proto)
			switch {
			case len(srcs) == 0:
				state = "[sem regra UFW]"
			case len(srcs) == 1 && srcs[0] == "Anywhere":
				state = "ABERTA a todos"
			default:
				state = "restrita: " + strings.Join(srcs, ", ")
			}
		}
		fmt.Printf("%-12s %-20s %s\n", fmt.Sprintf("%d/%s", p.Port, p.Proto), p.Label, state)
	}
	return nil
}
