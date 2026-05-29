package state

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PortKind classifies a managed port so the firewall layer knows which
// chain the rule belongs to:
//
//   - "host" — service listening on the host itself (sshd). UFW INPUT chain.
//   - "docker" — port published by a docker container (`docker run -p ...`).
//     Docker DNATs to the container, so the traffic hits FORWARD via the
//     DOCKER-USER chain and INPUT-targeted UFW rules don't apply. Uses
//     `ufw route allow ...` (FORWARD).
const (
	KindHost   = "host"
	KindDocker = "docker"
)

type ManagedPort struct {
	Port  int    `yaml:"port"`
	Proto string `yaml:"proto"`
	Label string `yaml:"label"`
	// Kind is "host" or "docker". Empty values from old state files are
	// treated as host by the firewall layer (back-compat: pre-Kind ports
	// were all host-listening services like sshd).
	Kind string `yaml:"kind,omitempty"`
}

// EffectiveKind returns Kind with the empty-string fallback applied.
func (p ManagedPort) EffectiveKind() string {
	if p.Kind == "" {
		return KindHost
	}
	return p.Kind
}

type State struct {
	ManagedPorts     []ManagedPort `yaml:"managed_ports"`
	InstalledModules []string      `yaml:"installed_modules"`

	path string `yaml:"-"`
}

// Path returns the state file path: same dir as config, named state.yaml.
func Path(cfgPath string) string {
	return filepath.Join(filepath.Dir(cfgPath), "state.yaml")
}

func Load(cfgPath string) (*State, error) {
	p := Path(cfgPath)
	s := &State{path: p}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, s); err != nil {
		return nil, err
	}
	s.path = p
	return s, nil
}

func (s *State) Save() error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

// HasPort reports whether the given port/proto is registered.
func (s *State) HasPort(port int, proto string) bool {
	for _, e := range s.ManagedPorts {
		if e.Port == port && e.Proto == proto {
			return true
		}
	}
	return false
}

// AddPort registers a managed port if not already present.
func (s *State) AddPort(p ManagedPort) bool {
	for _, e := range s.ManagedPorts {
		if e.Port == p.Port && e.Proto == p.Proto {
			return false
		}
	}
	s.ManagedPorts = append(s.ManagedPorts, p)
	return true
}

// RemovePort drops a tracked port (e.g. when feature is disabled).
func (s *State) RemovePort(port int, proto string) bool {
	for i, e := range s.ManagedPorts {
		if e.Port == port && e.Proto == proto {
			s.ManagedPorts = append(s.ManagedPorts[:i], s.ManagedPorts[i+1:]...)
			return true
		}
	}
	return false
}

// MarkInstalled records that a module has been run successfully by erp24.
// Returns true if newly added.
func (s *State) MarkInstalled(name string) bool {
	for _, n := range s.InstalledModules {
		if n == name {
			return false
		}
	}
	s.InstalledModules = append(s.InstalledModules, name)
	return true
}

// IsInstalled reports whether a module was previously run by erp24.
func (s *State) IsInstalled(name string) bool {
	for _, n := range s.InstalledModules {
		if n == name {
			return true
		}
	}
	return false
}
