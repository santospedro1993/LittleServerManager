package sysctl

import (
	"os"
	"strings"

	"lsm/internal/runner"
)

const ConfPath = "/etc/sysctl.d/99-lsm.conf"

const Body = `# Managed by lsm — do not edit by hand.

# --- Network hardening ---
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.all.log_martians = 1
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.icmp_ignore_bogus_error_responses = 1
net.ipv4.tcp_syncookies = 1

# --- Forwarding (Docker / containers) ---
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1

# --- TCP perf ---
net.core.somaxconn = 1024
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_keepalive_time = 600

# --- VM ---
vm.swappiness = 10
vm.dirty_ratio = 20
vm.dirty_background_ratio = 5

# --- fs (containers abrem muitos fd) ---
fs.file-max = 2097152
fs.inotify.max_user_watches = 524288
fs.inotify.max_user_instances = 512
`

// Expected returns the key→value pairs encoded in Body, for validation.
func Expected() map[string]string {
	m := map[string]string{}
	for _, ln := range strings.Split(Body, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		parts := strings.SplitN(ln, "=", 2)
		if len(parts) != 2 {
			continue
		}
		m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return m
}

func Write() error {
	return os.WriteFile(ConfPath, []byte(Body), 0644)
}

func Apply() error {
	return runner.Run("sysctl", "--system")
}

func Get(key string) (string, error) {
	out, err := runner.Capture("sysctl", "-n", key)
	return strings.TrimSpace(out), err
}
