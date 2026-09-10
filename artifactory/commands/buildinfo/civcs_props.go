package buildinfo

import (
	"strings"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
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
	setPropsWithRetry(servicesManager, buildSpecFromPaths(artifactPaths), props)
}

// setPropsViaBuildSearch resolves the build's artifacts through the dedicated build-artifacts API
// (indexed, no AQL JOINs, no OriginalDeploymentRepo dependency) and sets VCS properties on them.
// Used only when >=1 artifact is missing OriginalDeploymentRepo. Never fails the build.
func setPropsViaBuildSearch(servicesManager artifactory.ArtifactoryServicesManager, buildName, buildNumber, project, props string) {
	if buildName == "" || buildNumber == "" {
		log.Debug("CI VCS: build name/number missing, skipping build-search tagging")
		return
	}
	specFiles, err := spec.CreateSpecFromBuildNameNumberAndProject(buildName, buildNumber, project)
	if err != nil {
		log.Warn("CI VCS: failed to build search spec:", err)
		return
	}
	setPropsWithRetry(servicesManager, specFiles, props)
}

// setPropsWithRetry executes SearchItems + SetProps with exponential-backoff retry.
// Stops early on 404 (path not found) or 403 after first retry. Never fails the build.
func setPropsWithRetry(sm artifactory.ArtifactoryServicesManager, specFiles *spec.SpecFiles, props string) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelayBase * time.Duration(1<<(attempt-1)))
		}
		reader, err := generic.SearchItems(specFiles, sm)
		if err != nil {
			lastErr = err
			continue
		}
		length, _ := reader.Length()
		if length == 0 {
			_ = reader.Close()
			return
		}
		successCount, err := sm.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true})
		_ = reader.Close()
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
	specFiles := &spec.SpecFiles{}
	for _, artifactPath := range artifactPaths {
		specFiles.Files = append(specFiles.Files, spec.File{
			Pattern: artifactPath,
		})
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
