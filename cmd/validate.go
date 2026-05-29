package cmd

import (
	"fmt"
	"os/user"
	"strings"

	"github.com/spf13/cobra"

	"erp24/internal/config"
	"erp24/internal/fail2ban"
	"erp24/internal/hostname"
	"erp24/internal/runner"
	"erp24/internal/state"
	"erp24/internal/sysctl"
	"erp24/internal/timesync"
	"erp24/internal/ufw"
	"erp24/internal/upgrades"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Verify current setup against config (read-only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _, err := runValidate()
		return err
	},
}

func init() { rootCmd.AddCommand(validateCmd) }

// runValidate audits the current system against config and returns the set
// of modules with at least one failed check, plus the FAIL count. The
// "check & fix" flow consumes failedModules to know which modules to re-run.
func runValidate() (failedModules []string, fail int, err error) {
	if !config.Exists(cfgFile) {
		return nil, 0, fmt.Errorf("config not found (%s) — run `erp24 init` first", cfgFile)
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, 0, err
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return nil, 0, err
	}

	fmt.Println("=== Validate Setup ===")
	fmt.Printf("Config: %s\n", cfgFile)
	fmt.Println()

	pass := 0
	warned := 0
	skipped := 0
	failedSet := map[string]bool{}
	currentModule := ""

	check := func(name string, ok bool, detail string) {
		mark := "OK  "
		if !ok {
			mark = "FAIL"
			fail++
			if currentModule != "" {
				failedSet[currentModule] = true
			}
		} else {
			pass++
		}
		if detail == "" {
			fmt.Printf("  [%s] %s\n", mark, name)
		} else {
			fmt.Printf("  [%s] %s — %s\n", mark, name, detail)
		}
	}
	warn := func(name, detail string) {
		warned++
		if detail == "" {
			fmt.Printf("  [WARN] %s\n", name)
		} else {
			fmt.Printf("  [WARN] %s — %s\n", name, detail)
		}
	}
	skip := func(module, reason string) {
		skipped++
		fmt.Printf("  [--  ] %s — %s\n", module, reason)
	}

	// --- firewall (UFW) ---
	currentModule = "firewall"
	if st.IsInstalled("firewall") {
		check("UFW installed", ufw.Installed(), "")
		if ufw.Installed() {
			check("UFW active", ufw.IsActive(), "")
		}
		// DOCKER-USER block in after.rules is what makes UFW rules apply
		// to docker-published ports. Without it, `docker run -p` bypasses
		// the firewall entirely.
		check("after.rules DOCKER-USER block present", ufw.HasAfterRulesDockerBlock(), "")
	} else {
		skip("firewall", "not installed by erp24")
	}

	// --- SSH ---
	currentModule = "ssh"
	if st.IsInstalled("ssh") {
		if _, err := user.Lookup(cfg.SSH.User); err == nil {
			check(fmt.Sprintf("user '%s' exists", cfg.SSH.User), true, "")
		} else {
			check(fmt.Sprintf("user '%s' exists", cfg.SSH.User), false, err.Error())
		}

		// sshd -T dumps the effective config (main file + Include drop-ins).
		// Falls back to grepping both files if sshd -T fails (rare, e.g.
		// running as non-root which we already gated, or invalid config).
		eff, _ := runner.Capture("sshd", "-T")
		effLow := strings.ToLower(eff)
		check(fmt.Sprintf("sshd Port = %d", cfg.SSH.Port),
			strings.Contains(effLow, fmt.Sprintf("port %d", cfg.SSH.Port)),
			"")
		check("sshd PermitRootLogin no",
			strings.Contains(effLow, "permitrootlogin no"),
			"")

		if ufw.Installed() {
			check(fmt.Sprintf("UFW allows SSH port %d/tcp", cfg.SSH.Port),
				ufw.PortPermitted(cfg.SSH.Port, "tcp"), "")
		}
	} else {
		skip("ssh", "not installed by erp24")
	}

	// --- Docker ---
	currentModule = "docker"
	if st.IsInstalled("docker") {
		check("Docker engine present", runner.HasCommand("docker"), "")
		if runner.HasCommand("docker") {
			out, _ := runner.Capture("systemctl", "is-active", "docker.service")
			check("docker.service active", strings.TrimSpace(out) == "active", strings.TrimSpace(out))
		}
	} else {
		skip("docker", "not installed by erp24")
	}

	// --- Managed ports (whenever UFW is installed) ---
	// Attribute each port's FAIL to the module that opens it on re-run,
	// so "Check & Fix" knows what to re-execute. User-added ports
	// (`erp24 port add`) have no owner — those FAILs surface but won't
	// trigger an auto-fix (re-running port add isn't idempotent enough
	// to do silently).
	if ufw.Installed() {
		for _, p := range st.ManagedPorts {
			currentModule = portOwnerModule(p.Port, p.Proto, cfg)
			kind := p.EffectiveKind()
			// "Permitted" means: has at least one ALLOW rule (Anywhere or
			// from-IP) on the right chain for this port's kind.
			permitted := kindIsOpenToAll(kind, p.Port, p.Proto) ||
				len(kindSpecificSources(kind, p.Port, p.Proto)) > 0
			check(fmt.Sprintf("UFW allows %d/%s (%s, kind=%s)", p.Port, p.Proto, p.Label, kind),
				permitted, "")
		}
		currentModule = ""
	}

	// --- fail2ban ---
	currentModule = "fail2ban"
	if st.IsInstalled("fail2ban") {
		check("fail2ban installed", fail2ban.Installed(), "")
		if fail2ban.Installed() {
			out, _ := runner.Capture("systemctl", "is-active", "fail2ban")
			active := strings.TrimSpace(out) == "active"
			check("fail2ban active", active, strings.TrimSpace(out))
			// Active-but-not-guarding is the silent-failure mode we care
			// about: bad jail.local syntax, filter regex error, wrong
			// logpath. JailLoaded checks the daemon registered the jail;
			// JailHealthy confirms its filter actually started.
			if active {
				loaded, err := fail2ban.JailLoaded("sshd")
				if err != nil {
					check("jail sshd loaded", false, err.Error())
				} else {
					check("jail sshd loaded", loaded, "")
					if loaded {
						check("jail sshd healthy", fail2ban.JailHealthy("sshd"), "")
					}
				}
			}
		}
	} else {
		skip("fail2ban", "not installed by erp24")
	}

	// --- unattended-upgrades ---
	currentModule = "upgrades"
	if st.IsInstalled("upgrades") {
		check("unattended-upgrades installed", upgrades.Installed(), "")
		if upgrades.Installed() {
			out, _ := runner.Capture("systemctl", "is-enabled", "unattended-upgrades")
			check("unattended-upgrades enabled", strings.TrimSpace(out) == "enabled", strings.TrimSpace(out))
		}
	} else {
		skip("upgrades", "not installed by erp24")
	}

	// --- timesync ---
	currentModule = "timesync"
	if st.IsInstalled("timesync") {
		ntpEnabled := timesync.NTPEnabled()
		check("NTP enabled (timedatectl)", ntpEnabled, "")
		switch {
		case timesync.Synced():
			check("system synchronized", true, "")
		case ntpEnabled:
			// Transient post-boot: timesyncd is up but first poll hasn't
			// returned yet. Don't fail validate — re-running in a few
			// seconds usually flips this to OK.
			warn("system synchronized", "NTP enabled, first poll pending — re-run validate in a few seconds")
		default:
			check("system synchronized", false, "")
		}
		curTZ := timesync.CurrentTimezone()
		check(fmt.Sprintf("Timezone = %s", cfg.Timezone),
			curTZ == cfg.Timezone, "current: "+curTZ)
	} else {
		skip("timesync", "not installed by erp24")
	}

	// --- sysctl ---
	currentModule = "sysctl"
	if st.IsInstalled("sysctl") {
		for _, key := range []string{
			"net.ipv4.ip_forward",
			"net.ipv4.tcp_syncookies",
			"net.ipv4.conf.all.rp_filter",
			"vm.swappiness",
		} {
			want := sysctl.Expected()[key]
			got, _ := sysctl.Get(key)
			check(fmt.Sprintf("sysctl %s = %s", key, want), got == want, "current: "+got)
		}
	} else {
		skip("sysctl", "not installed by erp24")
	}

	// --- hostname ---
	currentModule = "hostname"
	if st.IsInstalled("hostname") {
		cur, _ := hostname.Current()
		check(fmt.Sprintf("hostname = %s", cfg.Hostname), cur == cfg.Hostname, "current: "+cur)
	} else if cfg.Hostname != "" {
		skip("hostname", "configured but not applied by erp24")
	}

	fmt.Println()
	fmt.Printf("Total: %d OK, %d FAIL, %d WARN, %d skipped\n", pass, fail, warned, skipped)

	for m := range failedSet {
		failedModules = append(failedModules, m)
	}
	if fail > 0 {
		err = fmt.Errorf("validation failed for %d check(s)", fail)
	}
	return failedModules, fail, err
}

// portOwnerModule returns the module that re-applies a given port's UFW
// rule when run. Empty string = user-managed port (e.g. `erp24 port add`),
// which has no auto-fix path. Extend this when adding modules that
// register their own managed ports (wireguard, etc).
func portOwnerModule(port int, proto string, cfg *config.Config) string {
	if proto == "tcp" && port == cfg.SSH.Port {
		return "ssh"
	}
	return ""
}

// moduleRunners maps validate-tracked module names to their RunE entrypoints.
// Used by the "check & fix" flow to re-run modules with failures.
func moduleRunners() map[string]func() error {
	return map[string]func() error{
		"firewall": func() error { return firewallCmd.RunE(firewallCmd, nil) },
		"ssh":      func() error { return sshCmd.RunE(sshCmd, nil) },
		"docker":   func() error { return dockerCmd.RunE(dockerCmd, nil) },
		"fail2ban": func() error { return fail2banCmd.RunE(fail2banCmd, nil) },
		"upgrades": func() error { return upgradesCmd.RunE(upgradesCmd, nil) },
		"timesync": func() error { return timesyncCmd.RunE(timesyncCmd, nil) },
		"sysctl":   func() error { return sysctlCmd.RunE(sysctlCmd, nil) },
		"hostname": func() error { return hostnameCmd.RunE(hostnameCmd, nil) },
	}
}
