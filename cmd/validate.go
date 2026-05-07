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
		return runValidate()
	},
}

func init() { rootCmd.AddCommand(validateCmd) }

func runValidate() error {
	if !config.Exists(cfgFile) {
		return fmt.Errorf("config não existe (%s) — corre 'lsm init' primeiro", cfgFile)
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	st, err := state.Load(cfgFile)
	if err != nil {
		return err
	}

	fmt.Println("=== Validar Setup ===")
	fmt.Printf("Config: %s\n", cfgFile)
	fmt.Println()

	pass := 0
	fail := 0
	check := func(name string, ok bool, detail string) {
		mark := "OK  "
		if !ok {
			mark = "FAIL"
			fail++
		} else {
			pass++
		}
		if detail == "" {
			fmt.Printf("  [%s] %s\n", mark, name)
		} else {
			fmt.Printf("  [%s] %s — %s\n", mark, name, detail)
		}
	}

	// --- UFW ---
	check("UFW instalado", ufw.Installed(), "")
	if ufw.Installed() {
		check("UFW ativo", ufw.IsActive(), "")
	}

	// --- SSH ---
	if _, err := user.Lookup(cfg.SSH.User); err == nil {
		check(fmt.Sprintf("user '%s' existe", cfg.SSH.User), true, "")
	} else {
		check(fmt.Sprintf("user '%s' existe", cfg.SSH.User), false, err.Error())
	}

	out, _ := runner.Capture("grep", "-E", "^Port ", "/etc/ssh/sshd_config")
	check(fmt.Sprintf("sshd Port = %d", cfg.SSH.Port),
		strings.Contains(out, fmt.Sprintf("Port %d", cfg.SSH.Port)), strings.TrimSpace(out))

	out, _ = runner.Capture("grep", "-E", "^PermitRootLogin", "/etc/ssh/sshd_config")
	check("sshd PermitRootLogin no",
		strings.Contains(out, "PermitRootLogin no"), strings.TrimSpace(out))

	if ufw.Installed() {
		check(fmt.Sprintf("UFW permite porta SSH %d/tcp", cfg.SSH.Port),
			ufw.PortPermitted(cfg.SSH.Port, "tcp"), "")
	}

	// --- Docker ---
	if _, err := user.Lookup(cfg.Docker.RootlessUser); err == nil {
		check(fmt.Sprintf("user '%s' existe", cfg.Docker.RootlessUser), true, "")
	} else {
		check(fmt.Sprintf("user '%s' existe", cfg.Docker.RootlessUser), false, err.Error())
	}
	subuid, _ := runner.Capture("grep", fmt.Sprintf("^%s:", cfg.Docker.RootlessUser), "/etc/subuid")
	check("subuid configurado", strings.TrimSpace(subuid) != "", strings.TrimSpace(subuid))

	check("Docker engine presente", runner.HasCommand("docker"), "")

	// --- Managed ports ---
	if ufw.Installed() {
		for _, p := range st.ManagedPorts {
			check(fmt.Sprintf("UFW permite %d/%s (%s)", p.Port, p.Proto, p.Label),
				ufw.PortPermitted(p.Port, p.Proto), "")
		}
	}

	// --- fail2ban ---
	check("fail2ban instalado", fail2ban.Installed(), "")
	if fail2ban.Installed() {
		out, _ := runner.Capture("systemctl", "is-active", "fail2ban")
		check("fail2ban ativo", strings.TrimSpace(out) == "active", strings.TrimSpace(out))
	}

	// --- unattended-upgrades ---
	check("unattended-upgrades instalado", upgrades.Installed(), "")
	if upgrades.Installed() {
		out, _ := runner.Capture("systemctl", "is-enabled", "unattended-upgrades")
		check("unattended-upgrades enabled", strings.TrimSpace(out) == "enabled", strings.TrimSpace(out))
	}

	// --- timesync ---
	check("NTP enabled (timedatectl)", timesync.NTPEnabled(), "")
	check("Sistema sincronizado", timesync.Synced(), "")
	curTZ := timesync.CurrentTimezone()
	check(fmt.Sprintf("Timezone = %s", cfg.Timezone),
		curTZ == cfg.Timezone, "atual: "+curTZ)

	// --- sysctl (spot-check key values) ---
	for _, key := range []string{
		"net.ipv4.ip_forward",
		"net.ipv4.tcp_syncookies",
		"net.ipv4.conf.all.rp_filter",
		"vm.swappiness",
	} {
		want := sysctl.Expected()[key]
		got, _ := sysctl.Get(key)
		check(fmt.Sprintf("sysctl %s = %s", key, want), got == want, "atual: "+got)
	}

	// --- hostname (skip se não configurado) ---
	if cfg.Hostname != "" {
		cur, _ := hostname.Current()
		check(fmt.Sprintf("hostname = %s", cfg.Hostname), cur == cfg.Hostname, "atual: "+cur)
	}

	fmt.Println()
	fmt.Printf("Total: %d OK, %d FAIL\n", pass, fail)
	if fail > 0 {
		return fmt.Errorf("validação falhou em %d check(s)", fail)
	}
	return nil
}
