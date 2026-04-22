package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DetectComposeFile returns the path to the repo's compose file, or ("", false) if none found.
func DetectComposeFile(repoDir string) (string, bool) {
	for _, name := range []string{"compose.yml", "compose.yaml", "docker-compose.yml", "docker-compose.yaml"} {
		p := filepath.Join(repoDir, name)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// ParsedCompose is a minimal view of a compose file.
type ParsedCompose struct {
	Services map[string]ServiceSpec `yaml:"services"`
	// Preserve service declaration order.
	serviceOrder []string
}

type ServiceSpec struct {
	Image  string                 `yaml:"image,omitempty"`
	Build  any                    `yaml:"build,omitempty"`
	Labels any                    `yaml:"labels,omitempty"`
	Ports  []string               `yaml:"ports,omitempty"`
	Raw    map[string]any         `yaml:",inline"`
}

func ParseComposeFile(path string) (*ParsedCompose, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseCompose(data)
}

// ParseCompose decodes top-level services and preserves order.
func ParseCompose(data []byte) (*ParsedCompose, error) {
	var top yaml.Node
	if err := yaml.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parse compose: %w", err)
	}
	result := &ParsedCompose{Services: map[string]ServiceSpec{}}
	if top.Kind != yaml.DocumentNode || len(top.Content) == 0 {
		return result, nil
	}
	root := top.Content[0]
	if root.Kind != yaml.MappingNode {
		return result, nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i]
		v := root.Content[i+1]
		if k.Value != "services" || v.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(v.Content); j += 2 {
			svcName := v.Content[j].Value
			svcNode := v.Content[j+1]
			var spec ServiceSpec
			if err := svcNode.Decode(&spec); err != nil {
				return nil, fmt.Errorf("service %s: %w", svcName, err)
			}
			result.Services[svcName] = spec
			result.serviceOrder = append(result.serviceOrder, svcName)
		}
	}
	return result, nil
}

func (p *ParsedCompose) Order() []string {
	out := make([]string, len(p.serviceOrder))
	copy(out, p.serviceOrder)
	return out
}

// ResolvePrimaryService picks the service to scale + label.
// Precedence: tcd.primary=true label > explicit override > first in order.
func ResolvePrimaryService(p *ParsedCompose, override string) (string, error) {
	// 1) Label marker.
	for _, name := range p.serviceOrder {
		spec := p.Services[name]
		if labelHasPrimary(spec.Labels) {
			return name, nil
		}
	}
	// 2) Override flag.
	if override != "" {
		if _, ok := p.Services[override]; !ok {
			return "", fmt.Errorf("service %q not found in compose file", override)
		}
		return override, nil
	}
	// 3) First service.
	if len(p.serviceOrder) == 0 {
		return "", fmt.Errorf("no services in compose file")
	}
	return p.serviceOrder[0], nil
}

func labelHasPrimary(labels any) bool {
	switch v := labels.(type) {
	case map[string]any:
		if s, ok := v["tcd.primary"]; ok {
			return truthy(s)
		}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			if s == "tcd.primary=true" || s == "tcd.primary=\"true\"" {
				return true
			}
		}
	}
	return false
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "True" || x == "TRUE" || x == "1"
	}
	return false
}
