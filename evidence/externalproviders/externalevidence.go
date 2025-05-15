package externalproviders

import (
	"gopkg.in/yaml.v3"
	"os"
)

type EvidenceProvider interface {
	// GetEvidence returns the evidence for the given type of external providers evidence
	GetEvidence() ([]byte, error)
}

func LoadConfig(path string) (map[string]*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	m := make(map[string]*yaml.Node)
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = *root.Content[0]
	}
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i].Value
		val := root.Content[i+1]
		m[key] = val
	}
	return m, nil
}
