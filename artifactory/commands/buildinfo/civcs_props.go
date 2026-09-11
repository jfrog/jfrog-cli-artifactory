package buildinfo

import (
	"strings"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	clientutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/http/jfroghttpclient"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const maxRetries = 3

// retryDelayBase is the base delay between retries. Exposed as a var so tests can zero it out.
var retryDelayBase = time.Second

// constructPresentArtifactPath returns "repo/path" when OriginalDeploymentRepo is present,
// or "" when it is missing (caller routes those to the build-artifacts search).
func constructPresentArtifactPath(artifact buildinfo.Artifact) string {
	if artifact.OriginalDeploymentRepo == "" {
		return ""
	}
	if artifact.Path != "" {
		return artifact.OriginalDeploymentRepo + "/" + strings.TrimPrefix(artifact.Path, "/")
	}
	if artifact.Name != "" {
		return artifact.OriginalDeploymentRepo + "/" + artifact.Name
	}
	return ""
}

// extractArtifactPaths splits build-info artifacts into direct paths (OriginalDeploymentRepo present)
// and a count of artifacts missing it (routed to the build-artifacts search).
func extractArtifactPaths(bi *buildinfo.BuildInfo) (present []string, missingCount int) {
	for _, m := range bi.Modules {
		for _, a := range m.Artifacts {
			if p := constructPresentArtifactPath(a); p != "" {
				present = append(present, p)
			} else {
				missingCount++
			}
		}
	}
	return present, missingCount
}

// setPropsOnArtifacts sets properties on artifacts with known exact paths.
func setPropsOnArtifacts(servicesManager artifactory.ArtifactoryServicesManager, artifactPaths []string, props string) {
	if len(artifactPaths) == 0 {
		return
	}
	setPropsViaSpecSearch(servicesManager, buildSpecFromPaths(artifactPaths), props)
}

// setPropsViaBuildSearch resolves the build's artifacts through the dedicated build-artifacts API
// (indexed, no AQL JOINs, no OriginalDeploymentRepo dependency) and sets VCS properties on them.
// Used only when >=1 artifact is missing OriginalDeploymentRepo. Never fails the build.
func setPropsViaBuildSearch(servicesManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project, props string) {
	if buildName == "" || buildNumber == "" {
		log.Debug("CI VCS: build name/number missing, skipping build-search tagging")
		return
	}
	setPropsFromSearchWithRetry(servicesManager, func() (*content.ContentReader, error) {
		return resolveBuildArtifacts(servicesManager, buildName, buildNumber, project)
	}, props)
}

// managerCommonConf adapts a services manager to the CommonConf the build-artifacts API expects,
// reusing the manager's already-configured HTTP client instead of building a new one.
type managerCommonConf struct {
	servicesManager artifactory.ArtifactoryServicesManager
}

func (c managerCommonConf) GetArtifactoryDetails() auth.ServiceDetails {
	return c.servicesManager.GetConfig().GetServiceDetails()
}

func (c managerCommonConf) GetJfrogHttpClient() *jfroghttpclient.JfrogHttpClient {
	return c.servicesManager.Client()
}

// resolveBuildArtifacts fetches a build's artifacts from the dedicated build-artifacts API.
// A spec-based build search would additionally fetch properties and checksums through AQL, which
// SetProps never reads. Declared as a var so tests can substitute it without a live Artifactory.
var resolveBuildArtifacts = func(servicesManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) (*content.ContentReader, error) {
	builds := []clientutils.Build{{BuildName: buildName, BuildNumber: buildNumber}}
	return clientutils.GetBuildArtifacts(builds, project, managerCommonConf{servicesManager})
}

// setPropsViaSpecSearch searches the given spec and sets props on the results.
func setPropsViaSpecSearch(sm artifactory.ArtifactoryServicesManager, specFiles *spec.SpecFiles, props string) {
	setPropsFromSearchWithRetry(sm, func() (*content.ContentReader, error) {
		return generic.SearchItems(specFiles, sm)
	}, props)
}

// setPropsFromSearchWithRetry resolves artifacts through searchArtifacts and sets props on them,
// with exponential-backoff retry. Stops early on 404 or 403-after-first-retry from SetProps.
// Never fails the build.
func setPropsFromSearchWithRetry(sm artifactory.ArtifactoryServicesManager, searchArtifacts func() (*content.ContentReader, error), props string) {
	closeReader := func(r *content.ContentReader) {
		if err := r.Close(); err != nil {
			log.Debug("CI VCS: failed to close search reader:", err)
		}
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelayBase * time.Duration(1<<(attempt-1)))
		}
		reader, err := searchArtifacts()
		if err != nil {
			// generic.SearchItems wraps all SearchFiles errors into a generic message,
			// so HTTP status codes (404, 403) cannot be detected here — always retry.
			lastErr = err
			continue
		}
		length, err := reader.Length()
		if err != nil {
			log.Debug("CI VCS: failed to read search result length:", err)
			closeReader(reader)
			lastErr = err
			continue
		}
		if length == 0 {
			closeReader(reader)
			return
		}
		successCount, err := sm.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true})
		closeReader(reader)
		if err == nil {
			log.Debug("CI VCS: set properties on", successCount, "artifacts")
			return
		}
		if is404Error(err) {
			return
		}
		if is403Error(err) && attempt >= 1 {
			return
		}
		lastErr = err
	}
	if lastErr != nil {
		log.Debug("CI VCS: failed to set properties after retries:", lastErr)
	}
}

// buildSpecFromPaths creates a SpecFiles object from artifact paths for search-based resolution.
func buildSpecFromPaths(artifactPaths []string) *spec.SpecFiles {
	specFiles := &spec.SpecFiles{Files: make([]spec.File, 0, len(artifactPaths))}
	for _, artifactPath := range artifactPaths {
		specFiles.Files = append(specFiles.Files, spec.File{Pattern: artifactPath})
	}
	return specFiles
}

// is404Error checks if the error indicates a 404 Not Found response.
func is404Error(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "not found")
}

// is403Error checks if the error indicates a 403 Forbidden response.
func is403Error(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "forbidden")
}
