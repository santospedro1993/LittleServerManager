package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"erp24/internal/config"
	"erp24/internal/prompt"
	"erp24/internal/runner"
	"erp24/internal/state"
	"erp24/internal/ufw"
)

var portCmd = &cobra.Command{
	Use:   "port",
	Short: "Manage UFW rules for arbitrary ports (per-port whitelist)",
}

var (
	portAddRestrict bool
	portAddHost     bool
)

var portAddCmd = &cobra.Command{
	Use:   "add <PORT>/<PROTO> [LABEL]",
	Short: "Register + open a port (default: docker-published, open to all; --host for host service, --restrict for no Anywhere rule)",
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
		kind := state.KindDocker
		if portAddHost {
			kind = state.KindHost
		}
		return portAdd(port, proto, label, kind, portAddRestrict)
	},
}

var portRemoveCmd = &cobra.Command{
	Use:   "remove <PORT>/<PROTO>",
	Short: "Close port + unregister from erp24 state",
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
	Short: "Revoke IP/CIDR for one port",
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
	portAddCmd.Flags().BoolVar(&portAddHost, "host", false,
		"port belongs to a host service (sshd-like, INPUT chain). Default: docker-published (FORWARD chain).")
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

// kindOf finds the Kind of a managed port in state. Returns KindHost when
// the port is unknown (safest fallback: host rules touch INPUT only and
// won't accidentally expose container traffic).
func kindOf(st *state.State, port int, proto string) string {
	for _, p := range st.ManagedPorts {
		if p.Port == port && p.Proto == proto {
			return p.EffectiveKind()
		}
	}
	return state.KindHost
}

// --- kind-dispatching UFW wrappers ---
//
// Host ports (sshd-like) need INPUT rules (`ufw allow ...`). Docker-published
// ports need FORWARD rules (`ufw route allow ...`) because dockerd DNATs the
// traffic to the container and INPUT never sees it. Lsm tags each managed
// port with a Kind at registration time so these helpers dispatch correctly.

func kindAllow(kind string, port int, proto, label string) error {
	if kind == state.KindDocker {
		return ufw.RouteAllow(port, proto, label)
	}
	return ufw.Allow(port, proto, label)
}

func kindAllowFrom(kind, ip string, port int, proto, label string) error {
	if kind == state.KindDocker {
		return ufw.RouteAllowFrom(ip, port, proto, label)
	}
	return ufw.AllowFrom(ip, port, proto, label)
}

func kindDeleteAllow(kind string, port int, proto string) error {
	if kind == state.KindDocker {
		return ufw.RouteDeleteAllow(port, proto)
	}
	return ufw.DeleteAllow(port, proto)
}

func kindDeleteAllowFrom(kind, ip string, port int, proto string) error {
	if kind == state.KindDocker {
		return ufw.RouteDeleteAllowFrom(ip, port, proto)
	}
	return ufw.DeleteAllowFrom(ip, port, proto)
}

func kindAllowedSources(kind string, port int, proto string) []string {
	if kind == state.KindDocker {
		return ufw.RouteAllowedSources(port, proto)
	}
	return ufw.AllowedSources(port, proto)
}

func kindSpecificSources(kind string, port int, proto string) []string {
	if kind == state.KindDocker {
		return ufw.RouteSpecificSources(port, proto)
	}
	return ufw.SpecificSources(port, proto)
}

func kindIsOpenToAll(kind string, port int, proto string) bool {
	if kind == state.KindDocker {
		return ufw.IsRouteOpenToAll(port, proto)
	}
	return ufw.IsOpenToAll(port, proto)
}

func portAdd(port int, proto, label, kind string, restrict bool) error {
	if !ufw.Installed() {
		return fmt.Errorf("UFW não instalado")
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	// If port already exists, preserve its kind (avoid silently flipping a
	// previously-host-tagged port to docker on re-run).
	existingKind := ""
	for _, p := range st.ManagedPorts {
		if p.Port == port && p.Proto == proto {
			existingKind = p.EffectiveKind()
			break
		}
	}
	if existingKind != "" {
		kind = existingKind
	}

	added := st.AddPort(state.ManagedPort{Port: port, Proto: proto, Label: label, Kind: kind})
	if added {
		if err := st.Save(); err != nil {
			return err
		}
		runner.Log("Porta %d/%s registada como '%s' (kind=%s).", port, proto, label, kind)
	} else {
		runner.Log("Porta %d/%s já estava registada (kind=%s).", port, proto, kind)
	}
	if restrict {
		runner.Log("--restrict: NÃO abro nada. Usa 'erp24 port allow %d/%s <IP>' para conceder acesso.", port, proto)
		return nil
	}
	if kindIsOpenToAll(kind, port, proto) || len(kindSpecificSources(kind, port, proto)) > 0 {
		runner.Log("UFW já tem regras para %d/%s (kind=%s) — mantido.", port, proto, kind)
		return nil
	}
	if err := kindAllow(kind, port, proto, label); err != nil {
		return err
	}
	runner.Log("UFW: %d/%s aberta a TODOS (kind=%s).", port, proto, kind)
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
	// Tenta remover em AMBAS as cadeias (docker/FWD + host/INPUT). Apagar uma
	// regra inexistente é inócuo; isto cobre o caso de o state ter perdido a
	// porta ou o kind ter desincronizado do UFW (senão a regra fica órfã).
	for _, kind := range []string{state.KindDocker, state.KindHost} {
		if kindIsOpenToAll(kind, port, proto) {
			_ = kindDeleteAllow(kind, port, proto)
		}
		for _, ip := range kindSpecificSources(kind, port, proto) {
			_ = kindDeleteAllowFrom(kind, ip, port, proto)
		}
	}
	// Confirma que o UFW ficou MESMO sem regras antes de mexer no state — não
	// mentir com "removida" se o delete falhou (era o bug do route delete).
	if ufw.IsOpenToAll(port, proto) || ufw.IsRouteOpenToAll(port, proto) ||
		len(ufw.SpecificSources(port, proto)) > 0 || len(ufw.RouteSpecificSources(port, proto)) > 0 {
		return fmt.Errorf("porta %d/%s ainda tem regras no UFW após remover — "+
			"corre 'sudo ufw status numbered' e apaga por número", port, proto)
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
		return fmt.Errorf("porta %d/%s não está em state — corre 'erp24 port add %d/%s' primeiro", port, proto, port, proto)
	}
	kind := kindOf(st, port, proto)
	for _, src := range kindSpecificSources(kind, port, proto) {
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
	if err := kindAllowFrom(kind, ip, port, proto, label); err != nil {
		return err
	}
	if kindIsOpenToAll(kind, port, proto) {
		_ = kindDeleteAllow(kind, port, proto)
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
	kind := kindOf(st, port, proto)
	remaining := 0
	for _, src := range kindSpecificSources(kind, port, proto) {
		if src != ip {
			remaining++
		}
	}
	willBeClosed := remaining == 0 && !kindIsOpenToAll(kind, port, proto)
	if willBeClosed && isSSHPort(port, proto) && !confirmCloseSSH(ip) {
		return nil
	}
	if err := kindDeleteAllowFrom(kind, ip, port, proto); err != nil {
		return err
	}
	runner.Log("UFW: %d/%s já não permite %s.", port, proto, ip)
	if len(kindSpecificSources(kind, port, proto)) == 0 && !kindIsOpenToAll(kind, port, proto) {
		runner.Log("UFW: %d/%s sem regras — porta fechada.", port, proto)
	}
	return nil
}

// isSSHPort reports whether port/proto matches the configured SSH port.
// Config-load errors → false (best-effort; we only use this for safety prompts).
func isSSHPort(port int, proto string) bool {
	if proto != "tcp" {
		return false
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return false
	}
	return cfg.SSH.Port == port
}

// confirmCloseSSH asks the admin to confirm closing the SSH port. Returns true
// if it's safe to proceed (--yes flag, or explicit confirmation). Lockout
// risk is real — without other access (console/IPMI), there's no way back in.
func confirmCloseSSH(ip string) bool {
	if yes {
		return true
	}
	fmt.Println()
	fmt.Printf("⚠ Revoking %s would leave the SSH port with NO rules.\n", ip)
	fmt.Println("  Result: SSH closed for everyone. Recovery needs console/IPMI access.")
	return prompt.Confirm("Proceed anyway?", false)
}

// portsLosingAllRules returns the managed ports that would end up rule-less
// if the given IP were revoked across all of them. Used to warn about SSH
// lockout before `remove-ip` deletes a whole batch of rules.
func portsLosingAllRules(st *state.State, ip string) []state.ManagedPort {
	var out []state.ManagedPort
	for _, p := range st.ManagedPorts {
		kind := p.EffectiveKind()
		if kindIsOpenToAll(kind, p.Port, p.Proto) {
			continue
		}
		remaining := 0
		for _, src := range kindSpecificSources(kind, p.Port, p.Proto) {
			if src != ip {
				remaining++
			}
		}
		if remaining == 0 {
			out = append(out, p)
		}
	}
	return out
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
	fmt.Println("PORT/PROTO   KIND     LABEL                ESTADO UFW")
	fmt.Println("───────────────────────────────────────────────────────────────────")
	for _, p := range st.ManagedPorts {
		stateStr := "[sem regra]"
		kind := p.EffectiveKind()
		if ufw.Installed() {
			srcs := kindAllowedSources(kind, p.Port, p.Proto)
			switch {
			case len(srcs) == 0:
				stateStr = "[sem regra UFW]"
			case len(srcs) == 1 && srcs[0] == "Anywhere":
				stateStr = "ABERTA a todos"
			default:
				stateStr = "restrita: " + strings.Join(srcs, ", ")
			}
		}
		fmt.Printf("%-12s %-8s %-20s %s\n",
			fmt.Sprintf("%d/%s", p.Port, p.Proto), kind, p.Label, stateStr)
	}
	return nil
}
