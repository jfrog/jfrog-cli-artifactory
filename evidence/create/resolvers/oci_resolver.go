package resolvers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/distribution/reference"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/auth"
)

// OciSubjectResolver handles resolution of Docker and OCI container image subjects
// Both Docker and OCI use the same container image format and registry protocols
type OciSubjectResolver struct {
	aqlResolver AqlResolver
	subject     string
	client      artifactory.ArtifactoryServicesManager
}

const registryHeader = "X-Artifactory-Docker-Registry"
const shaPrefix = "sha256:"

// Format: {protocol}://{registry}/v2/{path}/manifests/sha256:{checksum}
const registryUrlMask = "%s://%s/v2/%s/manifests/" + shaPrefix + "%s"

// NewOciSubjectResolver creates a new OciSubjectResolver instance
func NewOciSubjectResolver(subject string, client artifactory.ArtifactoryServicesManager) *OciSubjectResolver {
	return &OciSubjectResolver{
		subject:     subject,
		aqlResolver: NewAqlSubjectResolver(client),
		client:      client,
	}
}

// Resolve resolves a container image subject to repository paths using the checksum
func (d *OciSubjectResolver) Resolve(checksum string) ([]string, error) {
	if d.subject == "" {
		return nil, fmt.Errorf("subject cannot be empty")
	}
	if checksum == "" {
		return nil, fmt.Errorf("checksum cannot be empty")
	}
	if d.aqlResolver == nil || d.client == nil {
		return nil, fmt.Errorf("artifactory client is not properly initialized")
	}

	domain, path, err := parseOciSubject(d.subject)
	if err != nil {
		return nil, fmt.Errorf("failed to parse container subject: %w", err)
	}

	serviceDetails := d.client.GetConfig().GetServiceDetails()

	containerRegistryUrl, err := buildContainerUrl(domain, path, checksum, serviceDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to build container registry URL from subject: %s, %w", d.subject, err)
	}

	details := serviceDetails.CreateHttpClientDetails()
	resp, _, err := d.client.Client().SendHead(containerRegistryUrl, &details)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve container repository using URL: %s, %w", containerRegistryUrl, err)
	}

	repo := ""
	if values, ok := resp.Header[registryHeader]; ok && len(values) > 0 {
		repo = values[0]
	}

	if repo == "" {
		return nil, fmt.Errorf("no repository found in response headers for subject: %s", d.subject)
	}

	return d.aqlResolver.Resolve(repo, path, checksum)
}

// buildContainerUrl constructs a OCI registry v2 API URL for the given subject parts and checksum
func buildContainerUrl(domain, path, checksum string, serviceDetails auth.ServiceDetails) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if checksum == "" {
		return "", fmt.Errorf("checksum cannot be empty")
	}

	// Get the protocol and hostname from serviceDetails
	artifactoryURL := serviceDetails.GetUrl()
	if artifactoryURL == "" {
		return "", fmt.Errorf("artifactory URL is not configured")
	}

	parsedURL, err := url.Parse(artifactoryURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse artifactory URL '%s': %w", artifactoryURL, err)
	}

	// Use the protocol from serviceDetails and the domain from the subject
	protocol := parsedURL.Scheme
	if protocol == "" {
		protocol = "https" // Default to HTTPS if no protocol specified
	}

	registry := domain
	if registry == "" {
		registry = parsedURL.Hostname()
		if registry == "" {
			return "", fmt.Errorf("cannot determine registry hostname from subject or artifactory URL")
		}
	}

	normalized := strings.TrimPrefix(checksum, shaPrefix)

	manifestURL := fmt.Sprintf(registryUrlMask, protocol, registry, path, normalized)
	return manifestURL, nil
}

// parseOciSubject extracts registry domain and repository path from a OCI subject
func parseOciSubject(subject string) (string, string, error) {
	if subject == "" {
		return "", "", fmt.Errorf("subject cannot be empty")
	}

	ref, err := reference.Parse(subject)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse container reference '%s': %w", subject, err)
	}

	named, ok := ref.(reference.Named)
	if !ok {
		return "", "", fmt.Errorf("failed to parse container reference '%s': not a named reference", subject)
	}

	domain := reference.Domain(named)
	path := reference.Path(named)
	return domain, path, nil
}
