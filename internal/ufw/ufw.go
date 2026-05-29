package ufw

import (
	"fmt"
	"os"
	"strings"

	"erp24/internal/runner"
)

func Installed() bool { return runner.HasCommand("ufw") }

func Install() error {
	if Installed() {
		runner.Log("UFW já presente.")
		return nil
	}
	runner.Log("UFW não instalado — a instalar...")
	if err := runner.Run("apt-get", "update", "-qq"); err != nil {
		return err
	}
	return runner.Run("apt-get", "install", "-y", "-qq", "ufw")
}

func SetDefaults() error {
	if err := runner.Run("ufw", "default", "deny", "incoming"); err != nil {
		return err
	}
	return runner.Run("ufw", "default", "allow", "outgoing")
}

func Status() (string, error) { return runner.Capture("ufw", "status") }

func IsActive() bool {
	s, err := Status()
	if err != nil {
		return false
	}
	return strings.Contains(s, "Status: active")
}

func Enable() error {
	if IsActive() {
		runner.Log("UFW já ativo.")
		return nil
	}
	return runner.Run("ufw", "--force", "enable")
}

// PortPermitted returns true if "PORT/proto" is already allowed.
func PortPermitted(port int, proto string) bool {
	s, err := Status()
	if err != nil {
		return false
	}
	needle := fmt.Sprintf("%d/%s", port, proto)
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, needle) && strings.Contains(t, "ALLOW") {
			return true
		}
	}
	return false
}

// allowedSourcesForDirection parses `ufw status` and returns the From values
// that are allowed to reach PORT/proto on the chain indicated by `forward`:
//
//   - forward=false → INPUT chain (host services). Matches "ALLOW IN" and
//     bare "ALLOW" (default direction in older UFW output).
//   - forward=true  → FORWARD chain (docker-routed). Matches "ALLOW FWD".
//
// The string "Anywhere" indicates the port is open to all. IPv6 entries
// (lines containing "(v6)") are skipped to avoid duplicates — UFW mirrors
// v4 rules to v6 by default.
func allowedSourcesForDirection(port int, proto string, forward bool) []string {
	s, err := Status()
	if err != nil {
		return nil
	}
	needle := fmt.Sprintf("%d/%s", port, proto)
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, needle) {
			continue
		}
		if strings.Contains(t, "(v6)") {
			continue
		}
		isFwd := strings.Contains(t, "ALLOW FWD")
		if forward != isFwd {
			continue
		}
		if !strings.Contains(t, "ALLOW") {
			continue
		}
		// Trim everything up to and including the ACTION column.
		// Action column is one of: "ALLOW", "ALLOW IN", "ALLOW FWD".
		var marker string
		switch {
		case strings.Contains(t, "ALLOW FWD"):
			marker = "ALLOW FWD"
		case strings.Contains(t, "ALLOW IN"):
			marker = "ALLOW IN"
		default:
			marker = "ALLOW"
		}
		idx := strings.Index(t, marker)
		from := strings.TrimSpace(t[idx+len(marker):])
		if from == "" || seen[from] {
			continue
		}
		seen[from] = true
		out = append(out, from)
	}
	return out
}

// AllowedSources returns From values for PORT/proto on the INPUT chain
// (host services). For docker/forwarded rules, use RouteAllowedSources.
func AllowedSources(port int, proto string) []string {
	return allowedSourcesForDirection(port, proto, false)
}

// RouteAllowedSources returns From values for PORT/proto on the FORWARD
// chain (docker-routed traffic, via DOCKER-USER → ufw-user-forward).
func RouteAllowedSources(port int, proto string) []string {
	return allowedSourcesForDirection(port, proto, true)
}

// IsOpenToAll reports whether PORT/proto has an "Anywhere" allow rule on
// the INPUT chain.
func IsOpenToAll(port int, proto string) bool {
	for _, src := range AllowedSources(port, proto) {
		if src == "Anywhere" {
			return true
		}
	}
	return false
}

// IsRouteOpenToAll reports whether PORT/proto has an "Anywhere" allow rule
// on the FORWARD chain (docker-published port open to the world).
func IsRouteOpenToAll(port int, proto string) bool {
	for _, src := range RouteAllowedSources(port, proto) {
		if src == "Anywhere" {
			return true
		}
	}
	return false
}

// SpecificSources returns IPs/CIDRs allowed for PORT/proto on INPUT,
// excluding "Anywhere".
func SpecificSources(port int, proto string) []string {
	var out []string
	for _, src := range AllowedSources(port, proto) {
		if src != "Anywhere" {
			out = append(out, src)
		}
	}
	return out
}

// RouteSpecificSources returns IPs/CIDRs allowed for PORT/proto on FORWARD,
// excluding "Anywhere".
func RouteSpecificSources(port int, proto string) []string {
	var out []string
	for _, src := range RouteAllowedSources(port, proto) {
		if src != "Anywhere" {
			out = append(out, src)
		}
	}
	return out
}

// Allow opens a port from any source.
func Allow(port int, proto, comment string) error {
	return runner.Run("ufw", "allow",
		fmt.Sprintf("%d/%s", port, proto),
		"comment", comment)
}

// AllowFrom opens a port from a specific source IP/CIDR.
func AllowFrom(ip string, port int, proto, comment string) error {
	return runner.Run("ufw", "allow",
		"from", ip,
		"to", "any",
		"port", fmt.Sprintf("%d", port),
		"proto", proto,
		"comment", comment)
}

// DeleteAllowFrom removes a specific "allow from IP to any port PORT proto PROTO" rule.
func DeleteAllowFrom(ip string, port int, proto string) error {
	return runner.Run("ufw", "delete", "allow",
		"from", ip,
		"to", "any",
		"port", fmt.Sprintf("%d", port),
		"proto", proto)
}

// DeleteAllow removes a "allow PORT/proto" rule (any source).
func DeleteAllow(port int, proto string) error {
	return runner.Run("ufw", "delete", "allow", fmt.Sprintf("%d/%s", port, proto))
}

// RouteAllow opens a port from any source on the FORWARD chain (docker).
// Allows traffic to any docker container that DNATs PORT/proto.
func RouteAllow(port int, proto, comment string) error {
	return runner.Run("ufw", "route", "allow",
		"proto", proto,
		"from", "any",
		"to", "any",
		"port", fmt.Sprintf("%d", port),
		"comment", comment)
}

// RouteAllowFrom opens PORT/proto from a specific source on FORWARD.
func RouteAllowFrom(ip string, port int, proto, comment string) error {
	return runner.Run("ufw", "route", "allow",
		"proto", proto,
		"from", ip,
		"to", "any",
		"port", fmt.Sprintf("%d", port),
		"comment", comment)
}

// RouteDeleteAllowFrom removes a "route allow proto P from IP to any port N" rule.
func RouteDeleteAllowFrom(ip string, port int, proto string) error {
	return runner.Run("ufw", "delete", "route", "allow",
		"proto", proto,
		"from", ip,
		"to", "any",
		"port", fmt.Sprintf("%d", port))
}

// RouteDeleteAllow removes the "Anywhere" route allow rule for PORT/proto.
func RouteDeleteAllow(port int, proto string) error {
	return runner.Run("ufw", "delete", "route", "allow",
		"proto", proto,
		"from", "any",
		"to", "any",
		"port", fmt.Sprintf("%d", port))
}

// OpenWhitelisted opens port for each IP in allowedIPs, or to all if empty.
func OpenWhitelisted(port int, proto, label string, allowedIPs []string) error {
	if len(allowedIPs) == 0 {
		if err := Allow(port, proto, label); err != nil {
			return err
		}
		runner.Log("UFW: %d/%s aberta a TODOS (%s).", port, proto, label)
		return nil
	}
	for _, ip := range allowedIPs {
		if err := AllowFrom(ip, port, proto, fmt.Sprintf("%s - %s", label, ip)); err != nil {
			return err
		}
	}
	runner.Log("UFW: %d/%s permitida para %d IP(s) (%s).", port, proto, len(allowedIPs), label)
	return nil
}

// Reload reloads UFW so changes to /etc/ufw/*.rules take effect without a
// flush/re-enable cycle.
func Reload() error {
	return runner.Run("ufw", "reload")
}

// afterRulesPath is the file UFW concatenates after the user-defined rules.
const afterRulesPath = "/etc/ufw/after.rules"

// dockerBlockBegin / dockerBlockEnd bracket the erp24-managed block in
// /etc/ufw/after.rules. The block makes docker-published ports honour UFW
// FORWARD rules: DOCKER-USER jumps into ufw-user-forward (where `ufw route
// allow ...` rules live), lets internal docker networks RETURN early so
// container-to-container traffic still flows, and DROPs everything else.
//
// Source-of-pattern: the standard ufw-docker recipe (chaifeng/ufw-docker),
// adapted to a single inline block we own.
const (
	dockerBlockBegin = "# BEGIN ERP24-DOCKER"
	dockerBlockEnd   = "# END ERP24-DOCKER"
)

const dockerBlockBody = `# BEGIN ERP24-DOCKER
# Managed by erp24. Do not edit between BEGIN/END markers — run
# 'sudo erp24 firewall' to regenerate. Removing this block re-exposes
# every docker-published port to the world, bypassing UFW.
*filter
:ufw-user-forward - [0:0]
:DOCKER-USER - [0:0]
-A DOCKER-USER -j ufw-user-forward
-A DOCKER-USER -j RETURN -s 10.0.0.0/8
-A DOCKER-USER -j RETURN -s 172.16.0.0/12
-A DOCKER-USER -j RETURN -s 192.168.0.0/16
-A DOCKER-USER -j DROP
COMMIT
# END ERP24-DOCKER
`

// HasAfterRulesDockerBlock reports whether /etc/ufw/after.rules already
// contains the erp24-managed DOCKER-USER block.
func HasAfterRulesDockerBlock() bool {
	data, err := os.ReadFile(afterRulesPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), dockerBlockBegin) &&
		strings.Contains(string(data), dockerBlockEnd)
}

// WriteAfterRulesDockerBlock appends (or replaces) the erp24-managed
// DOCKER-USER block in /etc/ufw/after.rules. Idempotent: an existing block
// with the same body is left untouched, a stale block is replaced.
// Caller is responsible for calling Reload() afterwards.
func WriteAfterRulesDockerBlock() error {
	data, err := os.ReadFile(afterRulesPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", afterRulesPath, err)
	}
	body := string(data)

	beginIdx := strings.Index(body, dockerBlockBegin)
	endIdx := strings.Index(body, dockerBlockEnd)
	if beginIdx >= 0 && endIdx > beginIdx {
		// Replace existing block (covers stale erp24 versions).
		endLineEnd := endIdx + len(dockerBlockEnd)
		// Eat the trailing newline so we don't accumulate blanks.
		if endLineEnd < len(body) && body[endLineEnd] == '\n' {
			endLineEnd++
		}
		// Same-body short-circuit: nothing to do.
		current := body[beginIdx:endLineEnd]
		if strings.TrimSpace(current) == strings.TrimSpace(dockerBlockBody) {
			return nil
		}
		body = body[:beginIdx] + dockerBlockBody + body[endLineEnd:]
	} else {
		// Append at end of file. Ensure single trailing newline before block.
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += "\n" + dockerBlockBody
	}

	return os.WriteFile(afterRulesPath, []byte(body), 0640)
}

// RemoveAfterRulesDockerBlock strips the erp24-managed block from
// /etc/ufw/after.rules. Used only for teardown/uninstall paths.
func RemoveAfterRulesDockerBlock() error {
	data, err := os.ReadFile(afterRulesPath)
	if err != nil {
		return err
	}
	body := string(data)
	beginIdx := strings.Index(body, dockerBlockBegin)
	endIdx := strings.Index(body, dockerBlockEnd)
	if beginIdx < 0 || endIdx < beginIdx {
		return nil
	}
	endLineEnd := endIdx + len(dockerBlockEnd)
	if endLineEnd < len(body) && body[endLineEnd] == '\n' {
		endLineEnd++
	}
	// Also eat the blank line we inserted in front of the block.
	if beginIdx > 0 && body[beginIdx-1] == '\n' && beginIdx >= 2 && body[beginIdx-2] == '\n' {
		beginIdx--
	}
	body = body[:beginIdx] + body[endLineEnd:]
	return os.WriteFile(afterRulesPath, []byte(body), 0640)
}
