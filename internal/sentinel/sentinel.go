// Package sentinel installs and enrols the Sentinel monitoring agent
// (https://github.com/dev-erp24/Sentinel) on this host.
//
// Sentinel is pull-only: the agent dials OUT to a central dashboard over HTTPS
// and never listens on a port, so — unlike ssh/docker — this module opens NO
// UFW ports. Installing has two phases, both idempotent:
//
//  1. Package: download the agent .deb from {central}/downloads and apt-install
//     it (pulls in the systemd unit, the sentinel-agent binary and the dir
//     layout under /etc/sentinel + /var/lib/sentinel).
//  2. Enrolment: POST the operator's single-use install key to
//     {central}/api/agents/register, receive a permanent agent token, write
//     /etc/sentinel/agent.toml (0600) and start the service.
//
// The install key is a runtime secret (the operator pastes it from the
// dashboard) — never stored in erp24's config.yaml. Only the central URL,
// which is plain intent, lives there. If /etc/sentinel/agent.toml already
// exists the host is already enrolled: we skip enrolment so a re-run never
// burns a second install key.
package sentinel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"erp24/internal/prompt"
	"erp24/internal/runner"
)

// DefaultCentralURL matches the sentinel-installer default so operators see the
// same suggestion they'd get running the agent's own installer.
const DefaultCentralURL = "https://cc.erp24.pt"

const (
	agentBinPath    = "/usr/local/bin/sentinel-agent"
	agentConfigPath = "/etc/sentinel/agent.toml"
	serviceName     = "sentinel-agent.service"
)

// Installed reports whether the agent binary is present on the host.
func Installed() bool { return fileExists(agentBinPath) }

// Registered reports whether the host is already enrolled (agent.toml written).
func Registered() bool { return fileExists(agentConfigPath) }

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// NormalizeCentral cleans an operator-supplied central URL: it trims surrounding
// whitespace and any trailing slash and — because the dashboard is always served
// over HTTPS — prepends https:// when the operator typed a bare host with no
// scheme (e.g. "cc.erp24.pt" → "https://cc.erp24.pt"). An explicit http:// or
// https:// the operator wrote is left untouched. Empty in stays empty out.
func NormalizeCentral(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return strings.TrimRight(s, "/")
}

// Run installs the agent and enrols the host. central may be empty — the
// operator is then prompted (defaulting to DefaultCentralURL). provisioner is
// the erp24 version, sent as the initial agent_version to mark provenance.
func Run(central, provisioner string) error {
	central = NormalizeCentral(central)
	if central == "" {
		central = NormalizeCentral(prompt.Ask("Sentinel central URL", DefaultCentralURL))
	}
	if central == "" {
		return fmt.Errorf("sentinel: central URL is required")
	}

	runner.Section("Sentinel: install agent package")
	if err := installPackage(central); err != nil {
		return err
	}

	switch {
	case Registered():
		runner.Log("agent already enrolled (%s exists) — skipping registration (install key not reused)", agentConfigPath)
	case runner.DryRun:
		runner.Log("DRY: would prompt for an install key and POST %s/api/agents/register", central)
	default:
		runner.Section("Sentinel: register with central")
		key := askInstallKey()
		resp, err := register(central, key, provisioner)
		if err != nil {
			return fmt.Errorf("register: %w", err)
		}
		runner.Log("registered (agent_id=%s)", resp.AgentID)
		if err := writeAgentConfig(central, resp); err != nil {
			return fmt.Errorf("write %s: %w", agentConfigPath, err)
		}
		runner.Log("wrote %s (0600)", agentConfigPath)
	}

	runner.Section("Sentinel: enable service")
	return enableService()
}

// installPackage downloads the arch-matched agent .deb from the central's
// /downloads and installs it with apt (which resolves deps and re-runs cleanly).
func installPackage(central string) error {
	if runner.DryRun {
		runner.Log("DRY: would detect arch, download the agent .deb from %s/downloads/ and apt-get install", central)
		return nil
	}

	arch, err := runner.Capture("dpkg", "--print-architecture")
	if err != nil {
		return fmt.Errorf("detect architecture: %w", err)
	}
	arch = strings.TrimSpace(arch)

	// amd64 has no suffix (the agent's historical stable filename); arm64 does.
	var suffix string
	switch arch {
	case "amd64":
		suffix = ""
	case "arm64":
		suffix = "-arm64"
	default:
		return fmt.Errorf("unsupported architecture %q (only amd64/arm64 have agent packages)", arch)
	}

	url := fmt.Sprintf("%s/downloads/sentinel-agent%s.deb", central, suffix)
	tmp := "/tmp/sentinel-agent.deb"
	if err := runner.Run("curl", "-fsSL", "-o", tmp, url); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := runner.AptGet("install", "-y", tmp); err != nil {
		return err
	}
	runner.TryRun("rm", "-f", tmp)
	return nil
}

func askInstallKey() string {
	for {
		k := strings.TrimSpace(prompt.Ask("Install key (paste it from the Sentinel dashboard → Install keys)", ""))
		if k != "" {
			return k
		}
		fmt.Println("An install key is required. Create one in the dashboard and paste it here.")
	}
}

// --- registration protocol (mirrors agents/internal/transport) ---------------

type osInfo struct {
	Distro  string `json:"distro"`
	Version string `json:"version"`
	Kernel  string `json:"kernel,omitempty"`
	Arch    string `json:"arch,omitempty"`
}

type registerRequest struct {
	InstallKey   string   `json:"install_key"`
	Hostname     string   `json:"hostname"`
	Os           osInfo   `json:"os"`
	IPAddresses  []string `json:"ip_addresses,omitempty"`
	MacAddress   string   `json:"mac_address,omitempty"`
	AgentVersion string   `json:"agent_version"`
}

type registerResponse struct {
	AgentID                  string `json:"agent_id"`
	AgentToken               string `json:"agent_token"`
	CollectIntervalSeconds   int    `json:"collect_interval_seconds"`
	FlushIntervalSeconds     int    `json:"flush_interval_seconds"`
	PollLongWaitSeconds      int    `json:"poll_long_wait_seconds"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
}

func register(central, installKey, provisioner string) (*registerResponse, error) {
	host, _ := os.Hostname()
	body, err := json.Marshal(registerRequest{
		InstallKey:   installKey,
		Hostname:     host,
		Os:           detectOs(),
		IPAddresses:  detectIPs(),
		MacAddress:   detectMac(),
		AgentVersion: provisioner,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		central+"/api/agents/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sentinel-Version", provisioner)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("central returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var out registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if out.AgentToken == "" {
		return nil, fmt.Errorf("central returned an empty agent token")
	}
	return &out, nil
}

// writeAgentConfig renders /etc/sentinel/agent.toml exactly like the agent's own
// installer does (secure-by-default TLS, sqlite buffer, default docker socket).
// Written by hand — a template string keeps erp24 free of a TOML dependency.
func writeAgentConfig(central string, r *registerResponse) error {
	if err := os.MkdirAll("/etc/sentinel", 0o750); err != nil {
		return err
	}
	// 0600: the file holds the agent token.
	return os.WriteFile(agentConfigPath, []byte(renderAgentConfig(central, r)), 0o600)
}

// renderAgentConfig produces the /etc/sentinel/agent.toml body. Kept pure (no
// I/O) so it can be unit-tested against the agent's own config schema. Key names
// mirror agents/internal/config.Config exactly.
func renderAgentConfig(central string, r *registerResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[central]\n")
	fmt.Fprintf(&b, "url = %s\n", tomlStr(central))
	fmt.Fprintf(&b, "agent_id = %s\n", tomlStr(r.AgentID))
	fmt.Fprintf(&b, "agent_token = %s\n", tomlStr(r.AgentToken))
	fmt.Fprintf(&b, "verify_tls = true\n\n")
	fmt.Fprintf(&b, "[intervals]\n")
	fmt.Fprintf(&b, "collect_seconds = %d\n", r.CollectIntervalSeconds)
	fmt.Fprintf(&b, "flush_seconds = %d\n", r.FlushIntervalSeconds)
	fmt.Fprintf(&b, "poll_long_wait_seconds = %d\n", r.PollLongWaitSeconds)
	fmt.Fprintf(&b, "heartbeat_seconds = %d\n\n", r.HeartbeatIntervalSeconds)
	fmt.Fprintf(&b, "[buffer]\n")
	fmt.Fprintf(&b, "dir = \"/var/lib/sentinel/buffer\"\n")
	fmt.Fprintf(&b, "max_bytes = 134217728\n")
	fmt.Fprintf(&b, "backend = \"sqlite\"\n\n")
	fmt.Fprintf(&b, "[[docker]]\n")
	fmt.Fprintf(&b, "name = \"default\"\n")
	fmt.Fprintf(&b, "socket = \"/var/run/docker.sock\"\n\n")
	fmt.Fprintf(&b, "[host]\n")
	fmt.Fprintf(&b, "collect_per_core = true\n\n")
	fmt.Fprintf(&b, "[updates]\n")
	fmt.Fprintf(&b, "check_interval_seconds = 600\n")
	fmt.Fprintf(&b, "include_in_flush = true\n")
	return b.String()
}

// tomlStr renders a Go string as a TOML basic string, escaping the two
// characters that would break it. agent tokens are base64url so this only
// really matters for the URL, but it's cheap insurance.
func tomlStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func enableService() error {
	if err := runner.Run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := runner.Run("systemctl", "enable", serviceName); err != nil {
		return err
	}
	return runner.Run("systemctl", "restart", serviceName)
}

// --- host detection (mirrors the agent installer) ----------------------------

func detectOs() osInfo {
	info := osInfo{Arch: runtime.GOARCH}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			v = strings.Trim(v, `"`)
			switch k {
			case "ID":
				info.Distro = v
			case "VERSION_ID":
				info.Version = v
			}
		}
	}
	if info.Distro == "" {
		info.Distro = "linux"
	}
	if info.Version == "" {
		info.Version = "unknown"
	}
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		info.Kernel = strings.TrimSpace(string(data))
	}
	return info
}

func detectIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if v4 := ipNet.IP.To4(); v4 != nil {
					ips = append(ips, v4.String())
				}
			}
		}
	}
	return ips
}

func detectMac() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		if len(iface.HardwareAddr) > 0 {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}
