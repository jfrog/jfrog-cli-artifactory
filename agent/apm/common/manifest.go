package apmcommon

import (
	"net/url"
	"os"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"gopkg.in/yaml.v3"
)

const ApmManifestName = "apm.yml"

// ApmManifest represents apm.yml. Dependencies are intentionally not modeled here: the real
// schema is a nested map (dependencies: {apm: [...], mcp: [...]}), and nothing in this package
// needs the declared-dependency list, only name/version/registries.
//
// Registries is a map keyed by registry name, not a list.
type ApmManifest struct {
	Name       string             `yaml:"name"`
	Version    string             `yaml:"version"`
	Registries ManifestRegistries `yaml:"registries"`
}

// ManifestRegistries models apm.yml's registries: block: a map of registry name to entry, plus
// an optional sibling "default: <name>" key at the same YAML level as the registry names
// themselves - not nested under one - so it can't be a plain map[string]ManifestRegistry (see
// UnmarshalYAML).
type ManifestRegistries struct {
	Entries map[string]ManifestRegistry
	Default string
}

// UnmarshalYAML splits the "default" key out from the registry-name entries before decoding
// each one, so a real apm.yml with both (the schema-correct, common case) parses successfully
// instead of failing outright.
func (r *ManifestRegistries) UnmarshalYAML(value *yaml.Node) error {
	raw := make(map[string]yaml.Node)
	if err := value.Decode(&raw); err != nil {
		return err
	}
	r.Entries = make(map[string]ManifestRegistry, len(raw))
	for name, node := range raw {
		if name == "default" {
			if err := node.Decode(&r.Default); err != nil {
				return err
			}
			continue
		}
		var entry ManifestRegistry
		if err := node.Decode(&entry); err != nil {
			return err
		}
		r.Entries[name] = entry
	}
	return nil
}

type ManifestRegistry struct {
	URL string `yaml:"url"`
}

// LoadManifest reads and parses apm.yml at path. Every caller in this codebase constructs path
// from a working directory joined with the fixed ApmManifestName ("apm.yml"), never from
// unsanitized user input.
func LoadManifest(path string) (*ApmManifest, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is always workingDir+ApmManifestName, constructed by the caller, never user-supplied
	if err != nil {
		if os.IsNotExist(err) {
			return &ApmManifest{}, nil
		}
		return nil, errorutils.CheckError(err)
	}
	var manifest ApmManifest
	if err = yaml.Unmarshal(data, &manifest); err != nil {
		return nil, errorutils.CheckErrorf("parsing %s: %s", ApmManifestName, err.Error())
	}
	return &manifest, nil
}

// apmHostMatches returns true if registryURL and artifactoryURL share the same host.
func apmHostMatches(registryURL, artifactoryURL string) bool {
	rHost := parseHost(registryURL)
	aHost := parseHost(artifactoryURL)
	return rHost != "" && aHost != "" && rHost == aHost
}

func parseHost(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" {
		return ""
	}
	return strings.ToLower(parsedURL.Host)
}
