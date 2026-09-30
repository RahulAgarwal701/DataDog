// Package registry loads contracts/registry.yaml (service name <-> port mapping).
package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Name        string   `yaml:"name"`
	Port        int      `yaml:"port"`
	Kind        string   `yaml:"kind"` // "service" | "external"
	Description string   `yaml:"description"`
	DependsOn   []string `yaml:"depends_on"`
}

type file struct {
	Services []Service `yaml:"services"`
}

type Registry struct {
	services []Service
	byPort   map[int]int
	byName   map[string]int
}

// Load reads and validates a registry file.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	return Parse(data)
}

// Parse validates registry YAML content.
func Parse(data []byte) (*Registry, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	if len(f.Services) == 0 {
		return nil, fmt.Errorf("registry has no services")
	}
	r := &Registry{byPort: map[int]int{}, byName: map[string]int{}}
	for i, s := range f.Services {
		if s.Name == "" {
			return nil, fmt.Errorf("registry entry %d has no name", i)
		}
		if s.Port <= 0 || s.Port > 65535 {
			return nil, fmt.Errorf("service %q has invalid port %d", s.Name, s.Port)
		}
		if s.Kind == "" {
			s.Kind = "service"
		}
		if s.Kind != "service" && s.Kind != "external" {
			return nil, fmt.Errorf("service %q has invalid kind %q", s.Name, s.Kind)
		}
		if _, dup := r.byName[s.Name]; dup {
			return nil, fmt.Errorf("duplicate service name %q", s.Name)
		}
		if _, dup := r.byPort[s.Port]; dup {
			return nil, fmt.Errorf("duplicate port %d (service %q)", s.Port, s.Name)
		}
		r.services = append(r.services, s)
		r.byName[s.Name] = len(r.services) - 1
		r.byPort[s.Port] = len(r.services) - 1
	}
	return r, nil
}

// Services returns a copy of all registry entries, in file order.
func (r *Registry) Services() []Service {
	out := make([]Service, len(r.services))
	copy(out, r.services)
	return out
}

func (r *Registry) ByPort(port int) (Service, bool) {
	i, ok := r.byPort[port]
	if !ok {
		return Service{}, false
	}
	return r.services[i], true
}

func (r *Registry) ByName(name string) (Service, bool) {
	i, ok := r.byName[name]
	if !ok {
		return Service{}, false
	}
	return r.services[i], true
}

// KindOf returns the registry kind of a name, or "external" if it is unknown.
func (r *Registry) KindOf(name string) string {
	if s, ok := r.ByName(name); ok {
		return s.Kind
	}
	return "external"
}
