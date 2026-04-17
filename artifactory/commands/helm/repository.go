package helm

import (
	"fmt"
	"strings"
)

const (
	oci          = "oci://"
	schemeHttp   = "https"
	schemeSecure = "http"
)

// extractRepositoryNameFromURL extracts the repository name from an OCI or HTTPS URL.
// Supports both path-based access (oci://registry.com/repo-name) and
// subdomain access (oci://repo-name.registry.com) methods
func extractRepositoryNameFromURL(repositoryURL string) string {
	if repositoryURL == "" {
		return ""
	}
	if !strings.Contains(repositoryURL, "://") {
		return repositoryURL
	}
	repoURL := removeProtocolPrefix(repositoryURL)
	if repoURL == "" {
		return repositoryURL
	}
	parts := strings.Split(repoURL, "/")
	if len(parts) > 1 && parts[0] == "" {
		if len(parts) > 2 && parts[2] != "" {
			return parts[2]
		}
	} else if len(parts) > 1 && parts[1] != "" {
		return parts[1]
	}
	// No path segments found — try subdomain Docker access method where the
	// repository name is encoded as the first hostname label:
	// oci://demo-helm-local.jfrog.io  →  repo = "demo-helm-local"
	return extractRepositoryFromHostSubdomain(parts[0])
}

// extractRepositoryFromHostSubdomain extracts the repository name from the
// first label of a hostname used with Artifactory's subdomain Docker access method.
// For example, "demo-helm-local.jfrog.io" returns "demo-helm-local".
// Returns empty string if the hostname does not contain a subdomain.
func extractRepositoryFromHostSubdomain(host string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	labels := strings.SplitN(host, ".", 2)
	if len(labels) >= 2 && labels[0] != "" && labels[1] != "" {
		return labels[0]
	}
	return ""
}

type ociRepoCandidate struct {
	repoKey string
	subpath string
}

func generateRepoCandidates(registry, repository string) []ociRepoCandidate {
	if repository == "" {
		return []ociRepoCandidate{{repoKey: extractRepositoryFromHostSubdomain(registry)}}
	}
	segments := strings.FieldsFunc(repository, func(r rune) bool {
		return r == '/'
	})
	if len(segments) == 0 {
		return nil
	}
	candidates := []ociRepoCandidate{{repoKey: segments[0], subpath: strings.Join(segments[1:], "/")}}
	hostRepoKey := extractRepositoryFromHostSubdomain(registry)
	if hostRepoKey != "" && hostRepoKey != segments[0] {
		candidates = append(candidates, ociRepoCandidate{repoKey: hostRepoKey, subpath: repository})
	}
	return candidates
}

// removeProtocolPrefix removes protocol prefix from URL
func removeProtocolPrefix(repository string) string {
	prefixes := []string{oci, schemeHttp + "://", schemeSecure + "://"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(repository, prefix) {
			return strings.TrimPrefix(repository, prefix)
		}
	}
	parts := strings.Split(repository, "://")
	if len(parts) > 1 {
		return parts[1]
	}
	return repository
}

// extractDependencyPath extracts version path from dependency ID
func extractDependencyPath(depId string) string {
	parts := strings.Split(depId, ":")
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("%s/%s", parts[0], parts[1])
}

// isOCIRepository checks if a repository is OCI-compatible
func isOCIRepository(repository string) bool {
	if repository == "" {
		return false
	}
	return strings.HasPrefix(repository, oci)
}
