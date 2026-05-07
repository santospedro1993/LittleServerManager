package state

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ManagedPort struct {
	Port  int    `yaml:"port"`
	Proto string `yaml:"proto"`
	Label string `yaml:"label"`
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

// MarkInstalled records that a module has been run successfully by lsm.
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

// IsInstalled reports whether a module was previously run by lsm.
func (s *State) IsInstalled(name string) bool {
	for _, n := range s.InstalledModules {
		if n == name {
			return true
		}
	}
	return false
}
