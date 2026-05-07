package cmd

import (
	"fmt"
	"os"

	"lsm/internal/config"
	"lsm/internal/prompt"
	"lsm/internal/state"
)

// runMenu drives the interactive flow when lsm is invoked with no subcommand.
func runMenu() error {
	if !config.Exists(cfgFile) {
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
		fmt.Println("=========================================")
		fmt.Println("  lsm — config:", cfgFile)
		fmt.Println("=========================================")
		idx := prompt.Choose("Que ação?", []string{
			"Validar setup",
			"Correr módulo (firewall / ssh / docker / all)",
			"Adicionar IP à whitelist",
			"Remover IP da whitelist",
			"Ver IPs / portas geridas",
			"Re-correr Setup Inicial (sobrescreve config)",
			"Sair",
		})

		var err error
		switch idx {
		case 1:
			err = runValidate()
		case 2:
			err = chooseAndRunModule()
		case 3:
			ip := prompt.AskIPOrCIDR("IP/CIDR a adicionar")
			if ip == "" {
				continue
			}
			err = addIP(ip)
		case 4:
			err = interactiveRemoveIP()
		case 5:
			err = showOverview()
		case 6:
			err = runWizard()
		case 7:
			return nil
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
		}
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
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	if len(cfg.Network.AllowedIPs) == 0 {
		fmt.Println("Whitelist vazia.")
		return nil
	}
	opts := append(append([]string{}, cfg.Network.AllowedIPs...), "voltar")
	idx := prompt.Choose("Remover qual IP?", opts)
	if idx == len(opts) {
		return nil
	}
	return removeIP(cfg.Network.AllowedIPs[idx-1])
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
	fmt.Println("--- Whitelist (allowed_ips) ---")
	if len(cfg.Network.AllowedIPs) == 0 {
		fmt.Println("  (vazia → portas abrem a TODOS)")
	} else {
		for _, ip := range cfg.Network.AllowedIPs {
			fmt.Printf("  - %s\n", ip)
		}
	}
	fmt.Println()
	fmt.Println("--- Portas geridas (state.yaml) ---")
	if len(st.ManagedPorts) == 0 {
		fmt.Println("  (nenhuma)")
	} else {
		for _, p := range st.ManagedPorts {
			fmt.Printf("  - %d/%s  %s\n", p.Port, p.Proto, p.Label)
		}
	}
	fmt.Println()
	fmt.Println("--- Política ---")
	fmt.Println("  auto_open_ports:", cfg.Network.AutoOpenPorts)
	return nil
}
