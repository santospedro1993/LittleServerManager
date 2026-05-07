package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Timezone string        `yaml:"timezone"`
	Hostname string        `yaml:"hostname,omitempty"`
	FQDN     string        `yaml:"fqdn,omitempty"`
	SSH      SSHConfig     `yaml:"ssh"`
	Docker   DockerConfig  `yaml:"docker"`
	Network  NetworkConfig `yaml:"network"`

	path string `yaml:"-"`
}

type SSHConfig struct {
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type DockerConfig struct {
	RootlessUser string `yaml:"rootless_user"`
}

type NetworkConfig struct {
	AllowedIPs    []string `yaml:"allowed_ips"`
	AutoOpenPorts string   `yaml:"auto_open_ports"`
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

// AddAllowedIP appends ip if not present. Returns true if added.
func (c *Config) AddAllowedIP(ip string) bool {
	for _, e := range c.Network.AllowedIPs {
		if e == ip {
			return false
		}
	}
	c.Network.AllowedIPs = append(c.Network.AllowedIPs, ip)
	return true
}

// RemoveAllowedIP removes ip if present. Returns true if removed.
func (c *Config) RemoveAllowedIP(ip string) bool {
	for i, e := range c.Network.AllowedIPs {
		if e == ip {
			c.Network.AllowedIPs = append(c.Network.AllowedIPs[:i], c.Network.AllowedIPs[i+1:]...)
			return true
		}
	}
	return false
}
