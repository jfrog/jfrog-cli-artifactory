package resolvers

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-client-go/artifactory"
)

// SubjectResolver defines the interface for resolving subjects to repository paths
type SubjectResolver interface {
	Resolve(checksum string) ([]string, error)
}

// ResolverFunc is a function type that resolves subjects to repository paths
type ResolverFunc func(subject, checksum string, client artifactory.ArtifactoryServicesManager) ([]string, error)

// ociResolver handles OCI subject resolution
var ociResolver ResolverFunc = func(subject, checksum string, client artifactory.ArtifactoryServicesManager) ([]string, error) {
	resolver := NewOciSubjectResolver(subject, client)
	return resolver.Resolve(checksum)
}

// resolvers maps protocol prefixes to their corresponding resolver functions
var resolvers = map[string]ResolverFunc{
	"docker": ociResolver,
	"oci":    ociResolver,
}

// ResolveSubject resolves a subject to repository paths based on its protocol prefix
// The subject should be in the format "protocol://reference" (e.g., "docker://nginx:latest")
// If no protocol is specified or the protocol is not supported, returns the original subject
func ResolveSubject(subject, checksum string, client artifactory.ArtifactoryServicesManager) ([]string, error) {
	if subject == "" {
		return nil, fmt.Errorf("subject cannot be empty")
	}

	split := strings.Split(subject, "://")
	if len(split) != 2 {
		// No protocol specified, return original subject
		return []string{subject}, nil
	}

	typePrefix := strings.ToLower(strings.TrimSpace(split[0]))
	if typePrefix == "" {
		return nil, fmt.Errorf("protocol prefix cannot be empty")
	}

	if resolver, exists := resolvers[typePrefix]; exists {
		// Only validate client when we actually need it for resolution
		if client == nil {
			return nil, fmt.Errorf("artifactory client cannot be nil")
		}
		return resolver(split[1], checksum, client)
	}

	// Unsupported protocol, return original subject
	return []string{subject}, nil
}
