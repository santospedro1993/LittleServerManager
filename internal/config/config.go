package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents lsm intent — preferences and target values.
// It does NOT persist secrets (passwords) nor derived runtime data
// (effective UFW whitelists). Those live in state or the system itself.
type Config struct {
	Timezone string        `yaml:"timezone"`
	Hostname string        `yaml:"hostname,omitempty"`
	FQDN     string        `yaml:"fqdn,omitempty"`
	SSH      SSHConfig     `yaml:"ssh"`
	Docker   DockerConfig  `yaml:"docker"`
	Network  NetworkConfig `yaml:"network"`
	Modules  ModulesConfig `yaml:"modules"`

	path string `yaml:"-"`
}

// ModulesConfig records which optional modules the user opted into during
// the wizard. firewall + ssh are mandatory and not configurable.
// `lsm all` and the post-wizard auto-run respect these flags; individual
// `lsm <module>` invocations always run regardless.
type ModulesConfig struct {
	Firewall bool `yaml:"firewall"`
	SSH      bool `yaml:"ssh"`
	Sysctl   bool `yaml:"sysctl"`
	Timesync bool `yaml:"timesync"`
	Hostname bool `yaml:"hostname"`
	Docker   bool `yaml:"docker"`
	Fail2ban bool `yaml:"fail2ban"`
	Upgrades bool `yaml:"upgrades"`
}

type SSHConfig struct {
	Port int    `yaml:"port"`
	User string `yaml:"user"`
}

type DockerConfig struct {
	RootlessUser string `yaml:"rootless_user"`
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
	if c.Docker.RootlessUser == "" {
		return fmt.Errorf("config: docker.rootless_user required")
	}
	if c.Network.AutoOpenPorts == "" {
		c.Network.AutoOpenPorts = "ask"
	}
	if c.Timezone == "" {
		c.Timezone = "Europe/Lisbon"
	}
	// firewall + ssh are mandatory regardless of what the file says.
	c.Modules.Firewall = true
	c.Modules.SSH = true
	// Backward compat: configs written before the modules section existed
	// have all optional flags zeroed (false) after unmarshal. Treat the
	// "everything optional disabled" pattern as legacy and default it to
	// "everything enabled" so old configs don't silently lose modules.
	allOptionalFalse := !c.Modules.Sysctl && !c.Modules.Timesync && !c.Modules.Hostname &&
		!c.Modules.Docker && !c.Modules.Fail2ban && !c.Modules.Upgrades
	if allOptionalFalse {
		c.Modules.Sysctl = true
		c.Modules.Timesync = true
		c.Modules.Hostname = true
		c.Modules.Docker = true
		c.Modules.Fail2ban = true
		c.Modules.Upgrades = true
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
