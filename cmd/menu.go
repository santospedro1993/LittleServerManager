package cmd

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"erp24/internal/config"
	"erp24/internal/prompt"
	sshmod "erp24/internal/ssh"
	"erp24/internal/state"
	"erp24/internal/sysupdate"
)

// runMenu drives the interactive flow when erp24 is invoked with no subcommand.
// Menu options are filtered by role (admin = real root, operator = sudo from
// non-root). Submenus group related actions; numeric input picks an option,
// 'b' goes back, 'x' exits.
func runMenu() error {
	admin := RequireAdmin() == nil

	if !config.Exists(cfgFile) {
		if !admin {
			return fmt.Errorf("config not found at %s — initial setup requires direct root login", cfgFile)
		}
		return runBootstrap()
	}

	for {
		printHeader(admin)

		var labels []string
		var actions []func() (ran bool, err error)

		// always-available
		labels = append(labels, "Status")
		actions = append(actions, wrapSub(statusMenu))

		// Validate runs the full check; if anything failed AND caller is
		// admin, offers to re-run the failing modules. Operator-class
		// callers see the report and a hint to log in as root for fixes.
		labels = append(labels, "Validate")
		actions = append(actions, wrap(func() error { return runValidateAndMaybeFix(admin) }))

		labels = append(labels, "System")
		actions = append(actions, wrapSub(systemMenu))

		labels = append(labels, "Network")
		actions = append(actions, wrapSub(networkMenu(admin)))

		if admin {
			labels = append(labels, "Modules")
			actions = append(actions, wrapSub(modulesMenu))

			labels = append(labels, "Setup wizard")
			actions = append(actions, wrap(runWizard))
		}

		idx := prompt.ChooseEx("Action?", labels, false, true)
		if idx == prompt.ChoiceExit {
			return nil
		}
		// Wipe the menu screen so the action's output renders into a clean
		// buffer; the next menu iteration prints below the output.
		clearMenuScreen()

		ran, err := actions[idx-1]()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		if ran {
			fmt.Println("────────────────────────────────────────────")
			if !admin {
				fmt.Println("(destructive operations require direct root login)")
			}
			prompt.Pause("")
		}
	}
}

// runBootstrap is the first-run flow when no config exists. Steps:
//
//  1. apt update + upgrade + autoremove. If a reboot is required, prompt.
//     If user reboots, erp24 exits — they re-run after reboot and we resume
//     here (config still missing, so this flow re-enters at step 1, which
//     is now a no-op since the system is up to date).
//  2. Wizard (timezone, hostname, ssh user/port, docker user, modules).
//  3. Run selected modules.
//  4. Offer to make `sudo erp24` auto-launch on the SSH user's login.
func runBootstrap() error {
	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║  erp24 — first-run bootstrap                 ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Step 1/3: System update (apt update / upgrade / autoremove).")
	fmt.Println("This catches kernel/security patches before we configure anything.")
	if err := runSystemUpdate(); err != nil {
		return fmt.Errorf("system update failed: %w", err)
	}
	if need, _ := sysupdate.RebootRequired(); need {
		fmt.Println()
		fmt.Println("⚠ Reboot recommended before continuing setup.")
		fmt.Println("  After rebooting, re-run `sudo erp24` to resume here.")
		if err := promptReboot(); err != nil {
			return err
		}
		// Whether the user picked "now", "schedule", or "defer", we stop
		// the bootstrap here. Re-running erp24 later (no config) lands back
		// in this flow.
		return nil
	}

	fmt.Println()
	fmt.Println("Step 2/3: Wizard.")
	if err := runWizard(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Step 3/3: Run selected modules.")
	if !prompt.Confirm("Run them now?", true) {
		return nil
	}
	if err := runAllModules(); err != nil {
		return err
	}

	// Offer auto-launch. Loaded fresh because wizard saved cfg.SSH.User.
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil // setup succeeded; auto-launch is optional
	}
	fmt.Println()
	fmt.Println("─── Auto-launch on login ─────────────────────")
	fmt.Println("Add a snippet to ~/.profile so the erp24 menu opens automatically")
	fmt.Println("on interactive login. Press 'x' inside the menu to exit and")
	fmt.Println("drop into a regular shell.")
	fmt.Println()
	fmt.Printf("• %s (SSH operator)\n", cfg.SSH.User)
	if prompt.Confirm("Enable auto-launch for "+cfg.SSH.User+"?", true) {
		if err := sshmod.SetAutoLaunchERP24(cfg.SSH.User, true); err != nil {
			fmt.Fprintln(os.Stderr, "auto-launch:", err)
		}
	}
	fmt.Println()
	fmt.Println("• root (triggered by `su -`, `sudo -i`, or direct root login)")
	fmt.Println("  Plain `su` without `-` doesn't load /root/.profile, so the")
	fmt.Println("  menu won't fire there — use `su -` instead.")
	if prompt.Confirm("Enable auto-launch for root?", true) {
		if err := sshmod.SetAutoLaunchERP24("root", true); err != nil {
			fmt.Fprintln(os.Stderr, "auto-launch (root):", err)
		}
	}
	return nil
}

func printHeader(admin bool) {
	role := "operator (sudo)"
	if admin {
		role = "admin (root)"
	}
	line := fmt.Sprintf("erp24  ·  config: %s  ·  role: %s", cfgFile, role)
	border := strings.Repeat("─", utf8.RuneCountInString(line)+2)
	fmt.Println()
	fmt.Println("┌" + border + "┐")
	fmt.Println("│ " + line + " │")
	fmt.Println("└" + border + "┘")
}

// wrap turns a plain action into one that signals "did run, please pause".
func wrap(fn func() error) func() (bool, error) {
	return func() (bool, error) {
		fmt.Println()
		fmt.Println("─── output ─────────────────────────────────")
		return true, fn()
	}
}

// wrapSub runs a submenu function. Submenus handle their own pacing, so the
// outer loop should NOT pause after — that's why we return ran=false.
func wrapSub(fn func() error) func() (bool, error) {
	return func() (bool, error) { return false, fn() }
}

// statusMenu — host metrics. Read-only; available to admin + operator.
// Both modes return to the menu without a "press Enter to continue" pause.
// Each option clears the screen before running so the rendered frame
// occupies the whole terminal and the next menu prompt prints under it.
func statusMenu() error {
	for {
		idx := prompt.ChooseEx("Status", []string{
			"Snapshot",
			"Live",
		}, true, false)
		switch idx {
		case prompt.ChoiceBack:
			return nil
		case 1:
			clearMenuScreen()
			statusLive = false
			if err := statusOnce(); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
			printDivider()
		case 2:
			clearMenuScreen()
			statusLive = true
			err := statusLiveLoop()
			statusLive = false
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
			printDivider()
		}
	}
}

// printDivider draws a horizontal rule between command output and the next
// menu prompt. Used after non-paused actions (status, anywhere a Pause
// would feel redundant) so the menu doesn't appear glued to the output.
func printDivider() {
	fmt.Println()
	fmt.Println("────────────────────────────────────────────────────────")
}

// clearMenuScreen wipes the terminal between picking a menu option and the
// action running. The action's output then renders into a clean buffer and
// the next menu iteration prints right below it — matching the desired
// "previous menu disappears, action output stays, menu redraws below" UX.
func clearMenuScreen() { fmt.Print("\033[H\033[2J") }

// systemMenu — update + reboot. Available to both admin and operator.
func systemMenu() error {
	for {
		idx := prompt.ChooseEx("System", []string{
			"Update",
			"Reboot",
		}, true, false)
		switch idx {
		case prompt.ChoiceBack:
			return nil
		case 1:
			clearMenuScreen()
			if err := runSystemUpdate(); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
			printDivider()
		case 2:
			clearMenuScreen()
			if err := promptReboot(); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
			printDivider()
		}
	}
}

// networkMenu — ports list (read-only for operator), full management for admin.
func networkMenu(admin bool) func() error {
	return func() error {
		for {
			labels := []string{"List ports"}
			actions := []func() error{showOverview}

			if admin {
				labels = append(labels,
					"Add port",
					"Remove port",
					"Allow IP on port",
					"Revoke IP from port",
					"Allow IP on all ports",
					"Revoke IP from all ports",
				)
				actions = append(actions,
					interactivePortAdd,
					interactivePortRemove,
					interactivePortAllow,
					interactivePortRevoke,
					interactiveAllowAll,
					interactiveRevokeAll,
				)
			}

			idx := prompt.ChooseEx("Network", labels, true, false)
			if idx == prompt.ChoiceBack {
				return nil
			}
			clearMenuScreen()
			if err := actions[idx-1](); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
			}
			printDivider()
		}
	}
}

// modulesMenu — re-run any single module or `all`.
func modulesMenu() error {
	for {
		idx := prompt.ChooseEx("Modules", []string{
			"firewall",
			"ssh",
			"docker",
			"fail2ban",
			"upgrades",
			"timesync",
			"sysctl",
			"hostname",
			"all",
		}, true, false)
		if idx == prompt.ChoiceBack {
			return nil
		}
		clearMenuScreen()
		var err error
		switch idx {
		case 1:
			err = firewallCmd.RunE(firewallCmd, nil)
		case 2:
			err = sshCmd.RunE(sshCmd, nil)
		case 3:
			err = dockerCmd.RunE(dockerCmd, nil)
		case 4:
			err = fail2banCmd.RunE(fail2banCmd, nil)
		case 5:
			err = upgradesCmd.RunE(upgradesCmd, nil)
		case 6:
			err = timesyncCmd.RunE(timesyncCmd, nil)
		case 7:
			err = sysctlCmd.RunE(sysctlCmd, nil)
		case 8:
			err = hostnameCmd.RunE(hostnameCmd, nil)
		case 9:
			err = runAllModules()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		printDivider()
	}
}

// runValidateAndMaybeFix runs validate and, when there are failures and the
// caller is admin, offers to re-run the failing modules to apply fixes.
// Validate alone (without the fix path) is also reachable by operators —
// they see the same report plus a hint to escalate.
func runValidateAndMaybeFix(admin bool) error {
	failed, n, _ := runValidate()
	if n == 0 {
		fmt.Println("\nAll checks pass.")
		return nil
	}
	if !admin {
		fmt.Println("\nLog in as root to apply fixes (`erp24` from a root shell).")
		return nil
	}
	if len(failed) == 0 {
		fmt.Println("\nFailures detected but not tied to a known module — fix manually.")
		return nil
	}
	fmt.Printf("\nModules with failed checks: %v\n", failed)
	if !prompt.Confirm("Re-run those modules to apply fixes?", true) {
		return nil
	}
	runners := moduleRunners()
	for _, m := range failed {
		fn, ok := runners[m]
		if !ok {
			fmt.Printf("(no runner for module %q — skipped)\n", m)
			continue
		}
		fmt.Printf("\n=== Re-running %s ===\n", m)
		if err := fn(); err != nil {
			fmt.Fprintf(os.Stderr, "error in %s: %v\n", m, err)
		}
	}
	fmt.Println("\nRe-validating...")
	_, _, _ = runValidate()
	return nil
}

// --- interactive helpers used by networkMenu ---

func interactivePortAdd() error {
	spec := prompt.Ask("PORT/PROTO (e.g. 3306/tcp)", "")
	if spec == "" {
		return nil
	}
	port, proto, err := parsePortProto(spec)
	if err != nil {
		return err
	}
	label := prompt.Ask("Label", spec)
	// Caso comum: expor porta de container, aberta a todos. Menu assume isso.
	// host services (--host) e restrição por-IP (--restrict) ficam via CLI.
	return portAdd(port, proto, label, state.KindDocker, false)
}

func interactivePortRemove() error {
	port, proto, ok := pickManagedPort("Remove which port?")
	if !ok {
		return nil
	}
	if !prompt.Confirm(fmt.Sprintf("Confirm remove %d/%s?", port, proto), false) {
		return nil
	}
	return portRemove(port, proto)
}

func interactivePortAllow() error {
	port, proto, ok := pickManagedPort("Allow IP on which port?")
	if !ok {
		return nil
	}
	ip := prompt.AskIPOrCIDR("IP/CIDR to allow")
	if ip == "" {
		return nil
	}
	return portAllow(port, proto, ip)
}

func interactivePortRevoke() error {
	port, proto, ok := pickManagedPort("Revoke IP from which port?")
	if !ok {
		return nil
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}
	kind := kindOf(st, port, proto)
	srcs := kindSpecificSources(kind, port, proto)
	if len(srcs) == 0 {
		fmt.Println("No specific IPs — port is open to all or has no rules.")
		return nil
	}
	i := prompt.ChooseEx("Which IP to revoke?", srcs, true, false)
	if i == prompt.ChoiceBack {
		return nil
	}
	return portRevoke(port, proto, srcs[i-1])
}

func interactiveAllowAll() error {
	ip := prompt.AskIPOrCIDR("IP/CIDR to allow on ALL managed ports")
	if ip == "" {
		return nil
	}
	return addIP(ip)
}

func interactiveRevokeAll() error {
	cur := currentWhitelist()
	if len(cur) == 0 {
		fmt.Println("No specific IPs across managed ports.")
		return nil
	}
	idx := prompt.ChooseEx("Revoke which IP from ALL managed ports?", cur, true, false)
	if idx == prompt.ChoiceBack {
		return nil
	}
	return removeIP(cur[idx-1])
}

func pickManagedPort(question string) (int, string, bool) {
	st, err := state.Load(cfgFile)
	if err != nil || len(st.ManagedPorts) == 0 {
		fmt.Println("No managed ports.")
		return 0, "", false
	}
	opts := make([]string, 0, len(st.ManagedPorts))
	for _, p := range st.ManagedPorts {
		opts = append(opts, fmt.Sprintf("%d/%s — %s", p.Port, p.Proto, p.Label))
	}
	idx := prompt.ChooseEx(question, opts, true, false)
	if idx == prompt.ChoiceBack {
		return 0, "", false
	}
	p := st.ManagedPorts[idx-1]
	return p.Port, p.Proto, true
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
	fmt.Println("--- Managed ports + UFW state ---")
	if len(st.ManagedPorts) == 0 {
		fmt.Println("  (none — run modules first)")
	} else {
		for _, p := range st.ManagedPorts {
			kind := p.EffectiveKind()
			srcs := kindAllowedSources(kind, p.Port, p.Proto)
			if len(srcs) == 0 {
				fmt.Printf("  - %d/%s [%s]  %s — [no UFW rule]\n", p.Port, p.Proto, kind, p.Label)
				continue
			}
			fmt.Printf("  - %d/%s [%s]  %s\n", p.Port, p.Proto, kind, p.Label)
			for _, s := range srcs {
				if s == "Anywhere" {
					fmt.Printf("      • Anywhere (open to all)\n")
				} else {
					fmt.Printf("      • %s\n", s)
				}
			}
		}
	}

	fmt.Println()
	fmt.Println("--- Effective whitelist (union across managed ports) ---")
	wl := currentWhitelist()
	if len(wl) == 0 {
		fmt.Println("  (no specific IPs — ports open to all or no rules)")
	} else {
		for _, ip := range wl {
			fmt.Printf("  - %s\n", ip)
		}
	}

	fmt.Println()
	fmt.Println("--- Modules installed by erp24 ---")
	if len(st.InstalledModules) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, m := range st.InstalledModules {
			fmt.Printf("  - %s\n", m)
		}
	}

	fmt.Println()
	fmt.Println("--- Policy ---")
	fmt.Println("  auto_open_ports:", cfg.Network.AutoOpenPorts)
	return nil
}

// chooseAndRunModule and runAllModules retained for `erp24 all` and other callers.

// runAllModules runs the modules selected during the wizard, in safe order
// (firewall first, ssh after firewall, etc). Modules disabled in
// cfg.Modules are skipped silently — the user already chose during wizard.
func runAllModules() error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}

	type mod struct {
		name    string
		enabled bool
		fn      func() error
	}
	mods := []mod{
		{"firewall", cfg.Modules.Firewall, func() error { return firewallCmd.RunE(firewallCmd, nil) }},
		{"ssh", cfg.Modules.SSH, func() error { return sshCmd.RunE(sshCmd, nil) }},
		{"timesync", cfg.Modules.Timesync, func() error { return timesyncCmd.RunE(timesyncCmd, nil) }},
		{"sysctl", cfg.Modules.Sysctl, func() error { return sysctlCmd.RunE(sysctlCmd, nil) }},
		{"hostname", cfg.Modules.Hostname && cfg.Hostname != "", func() error { return hostnameCmd.RunE(hostnameCmd, nil) }},
		{"docker", cfg.Modules.Docker, func() error { return dockerCmd.RunE(dockerCmd, nil) }},
		{"fail2ban", cfg.Modules.Fail2ban, func() error { return fail2banCmd.RunE(fail2banCmd, nil) }},
		{"upgrades", cfg.Modules.Upgrades, func() error { return upgradesCmd.RunE(upgradesCmd, nil) }},
	}
	for _, m := range mods {
		if !m.enabled {
			fmt.Printf("[skip] %s — disabled in config.modules\n", m.name)
			continue
		}
		if err := m.fn(); err != nil {
			return fmt.Errorf("%s: %w", m.name, err)
		}
	}
	return nil
}
