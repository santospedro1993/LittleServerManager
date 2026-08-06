package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CurrentSchemaVersion is the latest schema understood by this build.
// Bump it whenever the YAML layout changes in a way that needs an
// in-memory migration on Load. A config file missing `schema_version`
// is treated as v0 (pre-versioning era).
const CurrentSchemaVersion = 1

// Config represents erp24 intent — preferences and target values.
// It does NOT persist secrets (passwords) nor derived runtime data
// (effective UFW whitelists). Those live in state or the system itself.
type Config struct {
	SchemaVersion int           `yaml:"schema_version"`
	Timezone      string        `yaml:"timezone"`
	Hostname      string        `yaml:"hostname,omitempty"`
	FQDN          string        `yaml:"fqdn,omitempty"`
	SSH           SSHConfig      `yaml:"ssh"`
	Network       NetworkConfig  `yaml:"network"`
	Sentinel      SentinelConfig `yaml:"sentinel,omitempty"`
	Modules       ModulesConfig  `yaml:"modules"`

	path string `yaml:"-"`
}

// ModulesConfig records which optional modules the user opted into during
// the wizard. firewall + ssh are mandatory and not configurable.
// `erp24 all` and the post-wizard auto-run respect these flags; individual
// `erp24 <module>` invocations always run regardless.
type ModulesConfig struct {
	Firewall bool `yaml:"firewall"`
	SSH      bool `yaml:"ssh"`
	Sysctl   bool `yaml:"sysctl"`
	Timesync bool `yaml:"timesync"`
	Hostname bool `yaml:"hostname"`
	Docker   bool `yaml:"docker"`
	Fail2ban bool `yaml:"fail2ban"`
	Upgrades bool `yaml:"upgrades"`
	// Sentinel is opt-in like Docker. Unlike the baseline modules it stays
	// false on legacy (schema v0) configs — a pre-existing install must not
	// silently enrol an agent (that needs an operator-issued install key).
	Sentinel bool `yaml:"sentinel"`
}

type SSHConfig struct {
	Port int    `yaml:"port"`
	User string `yaml:"user"`
}

// SentinelConfig holds the Sentinel monitoring agent settings. Only the central
// URL lives here (plain intent). The install key is a runtime secret and the
// agent token lives in /etc/sentinel/agent.toml — neither is ever in this file.
type SentinelConfig struct {
	CentralURL string `yaml:"central_url,omitempty"`
}

type NetworkConfig struct {
	AutoOpenPorts string `yaml:"auto_open_ports"`
}

// Exists reports whether the config file is present.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.path = path
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.SSH.Port == 0 {
		return fmt.Errorf("config: ssh.port required")
	}
	if c.SSH.User == "" {
		return fmt.Errorf("config: ssh.user required")
	}
	if c.Network.AutoOpenPorts == "" {
		c.Network.AutoOpenPorts = "ask"
	}
	if c.Timezone == "" {
		c.Timezone = "Etc/UTC"
	}
	// firewall + ssh are mandatory regardless of what the file says.
	c.Modules.Firewall = true
	c.Modules.SSH = true
	// Schema migration. SchemaVersion == 0 means the config was written
	// before erp24 started versioning the file. In that era, optional
	// modules weren't expressed in YAML — they were all implicitly on.
	// After unmarshal those fields are zero (false), so we flip them to
	// the legacy-era default (true) to avoid silently disabling modules
	// on existing installs. v1+ configs are honored as written.
	if c.SchemaVersion == 0 {
		c.Modules.Sysctl = true
		c.Modules.Timesync = true
		c.Modules.Hostname = true
		c.Modules.Docker = true
		c.Modules.Fail2ban = true
		c.Modules.Upgrades = true
		fmt.Fprintf(os.Stderr,
			"warning: %s has no schema_version (legacy config) — assuming all modules enabled.\n"+
				"         Re-run `erp24 init` or add `schema_version: %d` and the modules you actually want.\n",
			c.path, CurrentSchemaVersion)
		c.SchemaVersion = CurrentSchemaVersion
	}
	return nil
}

func (c *Config) Path() string     { return c.path }
func (c *Config) SetPath(p string) { c.path = p }

func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("config: no path set")
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600)
}
