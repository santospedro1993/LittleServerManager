package cmd

import (
	"fmt"
	"os/user"
	"strings"

	"github.com/spf13/cobra"

	"lsm/internal/config"
	"lsm/internal/fail2ban"
	"lsm/internal/hostname"
	"lsm/internal/runner"
	"lsm/internal/state"
	"lsm/internal/sysctl"
	"lsm/internal/timesync"
	"lsm/internal/ufw"
	"lsm/internal/upgrades"
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
		return nil, 0, fmt.Errorf("config not found (%s) — run `lsm init` first", cfgFile)
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
	} else {
		skip("firewall", "not installed by lsm")
	}

	// --- SSH ---
	currentModule = "ssh"
	if st.IsInstalled("ssh") {
		if _, err := user.Lookup(cfg.SSH.User); err == nil {
			check(fmt.Sprintf("user '%s' exists", cfg.SSH.User), true, "")
		} else {
			check(fmt.Sprintf("user '%s' exists", cfg.SSH.User), false, err.Error())
		}

		out, _ := runner.Capture("grep", "-E", "^Port ", "/etc/ssh/sshd_config")
		check(fmt.Sprintf("sshd Port = %d", cfg.SSH.Port),
			strings.Contains(out, fmt.Sprintf("Port %d", cfg.SSH.Port)), strings.TrimSpace(out))

		out, _ = runner.Capture("grep", "-E", "^PermitRootLogin", "/etc/ssh/sshd_config")
		check("sshd PermitRootLogin no",
			strings.Contains(out, "PermitRootLogin no"), strings.TrimSpace(out))

		if ufw.Installed() {
			check(fmt.Sprintf("UFW allows SSH port %d/tcp", cfg.SSH.Port),
				ufw.PortPermitted(cfg.SSH.Port, "tcp"), "")
		}
	} else {
		skip("ssh", "not installed by lsm")
	}

	// --- Docker ---
	currentModule = "docker"
	if st.IsInstalled("docker") {
		if _, err := user.Lookup(cfg.Docker.RootlessUser); err == nil {
			check(fmt.Sprintf("user '%s' exists", cfg.Docker.RootlessUser), true, "")
		} else {
			check(fmt.Sprintf("user '%s' exists", cfg.Docker.RootlessUser), false, err.Error())
		}
		subuid, _ := runner.Capture("grep", fmt.Sprintf("^%s:", cfg.Docker.RootlessUser), "/etc/subuid")
		check("subuid configured", strings.TrimSpace(subuid) != "", strings.TrimSpace(subuid))
		check("Docker engine present", runner.HasCommand("docker"), "")
	} else {
		skip("docker", "not installed by lsm")
	}

	// --- Managed ports (whenever UFW is installed) ---
	currentModule = ""
	if ufw.Installed() {
		for _, p := range st.ManagedPorts {
			check(fmt.Sprintf("UFW allows %d/%s (%s)", p.Port, p.Proto, p.Label),
				ufw.PortPermitted(p.Port, p.Proto), "")
		}
	}

	// --- fail2ban ---
	currentModule = "fail2ban"
	if st.IsInstalled("fail2ban") {
		check("fail2ban installed", fail2ban.Installed(), "")
		if fail2ban.Installed() {
			out, _ := runner.Capture("systemctl", "is-active", "fail2ban")
			check("fail2ban active", strings.TrimSpace(out) == "active", strings.TrimSpace(out))
		}
	} else {
		skip("fail2ban", "not installed by lsm")
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
		skip("upgrades", "not installed by lsm")
	}

	// --- timesync ---
	currentModule = "timesync"
	if st.IsInstalled("timesync") {
		check("NTP enabled (timedatectl)", timesync.NTPEnabled(), "")
		check("system synchronized", timesync.Synced(), "")
		curTZ := timesync.CurrentTimezone()
		check(fmt.Sprintf("Timezone = %s", cfg.Timezone),
			curTZ == cfg.Timezone, "current: "+curTZ)
	} else {
		skip("timesync", "not installed by lsm")
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
		skip("sysctl", "not installed by lsm")
	}

	// --- hostname ---
	currentModule = "hostname"
	if st.IsInstalled("hostname") {
		cur, _ := hostname.Current()
		check(fmt.Sprintf("hostname = %s", cfg.Hostname), cur == cfg.Hostname, "current: "+cur)
	} else if cfg.Hostname != "" {
		skip("hostname", "configured but not applied by lsm")
	}

	fmt.Println()
	fmt.Printf("Total: %d OK, %d FAIL, %d skipped\n", pass, fail, skipped)

	for m := range failedSet {
		failedModules = append(failedModules, m)
	}
	if fail > 0 {
		err = fmt.Errorf("validation failed for %d check(s)", fail)
	}
	return failedModules, fail, err
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
