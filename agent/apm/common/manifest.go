package apmcommon

import (
	"net/url"
	"os"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"gopkg.in/yaml.v3"
)

const ApmManifestName = "apm.yml"

// ApmManifest represents apm.yml. Dependencies are intentionally not modeled here:
// the real schema is a nested map (dependencies: {apm: [...], mcp: [...]}), and nothing
// in this package currently needs the declared-dependency list — only name/version/registries.
//
// Registries is a map keyed by registry name (confirmed live against a real apm.yml) —
// not a list. An earlier version of this struct modeled it as []ManifestRegistry, which
// made LoadManifest fail on every real apm.yml that declares any registries at all.
type ApmManifest struct {
	Name       string                      `yaml:"name"`
	Version    string                      `yaml:"version"`
	Registries map[string]ManifestRegistry `yaml:"registries"`
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
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}
