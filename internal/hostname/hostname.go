package hostname

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"erp24/internal/runner"
)

const HostsPath = "/etc/hosts"

func Set(name string) error {
	return runner.Run("hostnamectl", "set-hostname", name)
}

// UpdateHosts ensures /etc/hosts has:
//   - "127.0.0.1 localhost"   (preserved if present)
//   - "127.0.1.1 [<fqdn>] <short>"   (Debian convention; replaced or appended)
func UpdateHosts(short, fqdn string) error {
	data, err := os.ReadFile(HostsPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	mapping := "127.0.1.1\t" + short
	if fqdn != "" {
		mapping = "127.0.1.1\t" + fqdn + " " + short
	}

	re := regexp.MustCompile(`^\s*127\.0\.1\.1\b`)
	hasLocalhost := false
	replaced := false
	out := make([]string, 0, len(lines)+2)

	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "127.0.0.1") && strings.Contains(t, "localhost") {
			hasLocalhost = true
		}
		if re.MatchString(ln) {
			out = append(out, mapping)
			replaced = true
			continue
		}
		out = append(out, ln)
	}

	if !replaced {
		out = append(out, mapping)
	}
	if !hasLocalhost {
		out = append([]string{"127.0.0.1\tlocalhost"}, out...)
	}

	body := strings.Join(out, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return os.WriteFile(HostsPath, []byte(body), 0644)
}

func Current() (string, error) {
	out, err := runner.Capture("hostname")
	return strings.TrimSpace(out), err
}

func CurrentFQDN() string {
	out, _ := runner.Capture("hostname", "--fqdn")
	return strings.TrimSpace(out)
}

func Apply(short, fqdn string) error {
	if short == "" {
		return fmt.Errorf("hostname required")
	}
	if err := Set(short); err != nil {
		return err
	}
	return UpdateHosts(short, fqdn)
}
