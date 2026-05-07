package cmd

import (
	"fmt"
	"os"

	"lsm/internal/config"
	"lsm/internal/prompt"
	"lsm/internal/state"
	"lsm/internal/ufw"
)

// runMenu drives the interactive flow when lsm is invoked with no subcommand.
// Menu options are filtered by the caller's role: admin (real root login)
// sees everything; operator (sudo from non-root) sees only safe actions.
func runMenu() error {
	admin := RequireAdmin() == nil

	if !config.Exists(cfgFile) {
		if !admin {
			return fmt.Errorf("config não existe em %s — setup inicial requer login direto como root", cfgFile)
		}
		fmt.Println("Nenhuma config encontrada em", cfgFile)
		fmt.Println("→ A iniciar Setup Inicial.")
		fmt.Println()
		if err := runWizard(); err != nil {
			return err
		}
		if !prompt.Confirm("Correr 'firewall + ssh + docker' agora?", true) {
			return nil
		}
		return runAllModules()
	}

	for {
		fmt.Println()
		fmt.Println("╔═══════════════════════════════════════════╗")
		fmt.Println("║  lsm — menu                               ║")
		fmt.Printf("║  config: %-33s║\n", cfgFile)
		role := "operator (sudo)"
		if admin {
			role = "admin (root)"
		}
		fmt.Printf("║  role:   %-33s║\n", role)
		fmt.Println("╚═══════════════════════════════════════════╝")

		var options []string
		var actions []func() error
		options = append(options, "Validar setup")
		actions = append(actions, runValidate)
		options = append(options, "Atualizar servidor (apt update + upgrade + autoremove)")
		actions = append(actions, runUpdateServer)
		options = append(options, "Ver IPs / portas geridas")
		actions = append(actions, showOverview)

		if admin {
			options = append(options, "Correr módulo (firewall / ssh / docker / all)")
			actions = append(actions, chooseAndRunModule)
			options = append(options, "Adicionar IP à whitelist")
			actions = append(actions, func() error {
				ip := prompt.AskIPOrCIDR("IP/CIDR a adicionar")
				if ip == "" {
					return nil
				}
				return addIP(ip)
			})
			options = append(options, "Remover IP da whitelist")
			actions = append(actions, interactiveRemoveIP)
			options = append(options, "Re-correr Setup Inicial (sobrescreve config)")
			actions = append(actions, runWizard)
		}
		options = append(options, "Sair")

		idx := prompt.Choose("Que ação?", options)
		if idx == len(options) {
			return nil
		}

		fmt.Println()
		fmt.Println("─── output ─────────────────────────────────")
		err := actions[idx-1]()
		if err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
		}
		fmt.Println("────────────────────────────────────────────")
		if !admin {
			fmt.Println("(operações destrutivas requerem login direto como root)")
		}
		prompt.Pause("")
	}
}

func chooseAndRunModule() error {
	idx := prompt.Choose("Módulo", []string{
		"firewall  (UFW bootstrap)",
		"ssh       (hardening + abrir porta)",
		"docker    (install + rootless)",
		"fail2ban  (anti brute-force SSH)",
		"upgrades  (unattended security patches)",
		"timesync  (systemd-timesyncd + timezone)",
		"sysctl    (kernel hardening + tuning)",
		"hostname  (set hostname + /etc/hosts)",
		"all       (corre todos por ordem)",
		"voltar",
	})
	switch idx {
	case 1:
		return firewallCmd.RunE(firewallCmd, nil)
	case 2:
		return sshCmd.RunE(sshCmd, nil)
	case 3:
		return dockerCmd.RunE(dockerCmd, nil)
	case 4:
		return fail2banCmd.RunE(fail2banCmd, nil)
	case 5:
		return upgradesCmd.RunE(upgradesCmd, nil)
	case 6:
		return timesyncCmd.RunE(timesyncCmd, nil)
	case 7:
		return sysctlCmd.RunE(sysctlCmd, nil)
	case 8:
		return hostnameCmd.RunE(hostnameCmd, nil)
	case 9:
		return runAllModules()
	}
	return nil
}

func runAllModules() error {
	cfg, _ := config.Load(cfgFile)

	type mod struct {
		name string
		skip bool
		fn   func() error
	}
	mods := []mod{
		{name: "firewall", fn: func() error { return firewallCmd.RunE(firewallCmd, nil) }},
		{name: "timesync", fn: func() error { return timesyncCmd.RunE(timesyncCmd, nil) }},
		{name: "sysctl", fn: func() error { return sysctlCmd.RunE(sysctlCmd, nil) }},
		{name: "hostname", skip: cfg == nil || cfg.Hostname == "",
			fn: func() error { return hostnameCmd.RunE(hostnameCmd, nil) }},
		{name: "ssh", fn: func() error { return sshCmd.RunE(sshCmd, nil) }},
		{name: "docker", fn: func() error { return dockerCmd.RunE(dockerCmd, nil) }},
		{name: "fail2ban", fn: func() error { return fail2banCmd.RunE(fail2banCmd, nil) }},
		{name: "upgrades", fn: func() error { return upgradesCmd.RunE(upgradesCmd, nil) }},
	}
	for _, m := range mods {
		if m.skip {
			fmt.Printf("[skip] %s — não configurado\n", m.name)
			continue
		}
		if err := m.fn(); err != nil {
			return fmt.Errorf("%s: %w", m.name, err)
		}
	}
	return nil
}

func interactiveRemoveIP() error {
	cur := currentWhitelist()
	if len(cur) == 0 {
		fmt.Println("Nenhum IP específico em portas geridas (UFW).")
		return nil
	}
	opts := append(append([]string{}, cur...), "voltar")
	idx := prompt.Choose("Remover qual IP?", opts)
	if idx == len(opts) {
		return nil
	}
	return removeIP(cur[idx-1])
}

func showOverview() error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("--- Portas geridas + estado UFW ---")
	if len(st.ManagedPorts) == 0 {
		fmt.Println("  (nenhuma — corre os módulos primeiro)")
	} else {
		for _, p := range st.ManagedPorts {
			srcs := ufw.AllowedSources(p.Port, p.Proto)
			if len(srcs) == 0 {
				fmt.Printf("  - %d/%s  %s — [sem regra UFW]\n", p.Port, p.Proto, p.Label)
				continue
			}
			fmt.Printf("  - %d/%s  %s\n", p.Port, p.Proto, p.Label)
			for _, s := range srcs {
				if s == "Anywhere" {
					fmt.Printf("      • Anywhere (aberta a todos)\n")
				} else {
					fmt.Printf("      • %s\n", s)
				}
			}
		}
	}

	fmt.Println()
	fmt.Println("--- Whitelist efetiva (união entre portas geridas) ---")
	wl := currentWhitelist()
	if len(wl) == 0 {
		fmt.Println("  (sem IPs específicos — portas abertas a todos ou sem regras)")
	} else {
		for _, ip := range wl {
			fmt.Printf("  - %s\n", ip)
		}
	}

	fmt.Println()
	fmt.Println("--- Módulos instalados pelo lsm ---")
	if len(st.InstalledModules) == 0 {
		fmt.Println("  (nenhum)")
	} else {
		for _, m := range st.InstalledModules {
			fmt.Printf("  - %s\n", m)
		}
	}

	fmt.Println()
	fmt.Println("--- Política ---")
	fmt.Println("  auto_open_ports:", cfg.Network.AutoOpenPorts)
	return nil
}
