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
	ManagedPorts []ManagedPort `yaml:"managed_ports"`

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
