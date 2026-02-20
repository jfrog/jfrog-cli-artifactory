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

const (
	maxRetries     = 3
	retryDelayBase = time.Second
)

// extractArtifactPathsWithWarnings extracts Artifactory paths from build info artifacts.
// Returns the list of paths (may be complete or partial) and count of skipped artifacts.
// Paths are constructed using OriginalDeploymentRepo + Path when available, or Path directly as fallback.
// If property setting fails later due to incomplete paths, warnings will be logged at that point.
func extractArtifactPathsWithWarnings(buildInfo *buildinfo.BuildInfo) ([]string, int) {
	var paths []string
	var skippedCount int

	for _, module := range buildInfo.Modules {
		for _, artifact := range module.Artifacts {
			fullPath := constructArtifactPathWithFallback(artifact)
			if fullPath == "" {
				// No path information at all - skip silently (nothing to try)
				skippedCount++
				continue
			}
			paths = append(paths, fullPath)
		}
	}
	return paths, skippedCount
}

// constructArtifactPathWithFallback builds the full Artifactory path for an artifact.
// Strategy:
//  1. If OriginalDeploymentRepo is present: use OriginalDeploymentRepo + "/" + Path
//  2. If OriginalDeploymentRepo is missing: use Path directly (it may or may not work)
//  3. If neither available: return empty string (caller should warn and skip)
func constructArtifactPathWithFallback(artifact buildinfo.Artifact) string {
	// Primary: Use OriginalDeploymentRepo if available
	if artifact.OriginalDeploymentRepo != "" {
		if artifact.Path != "" {
			return artifact.OriginalDeploymentRepo + "/" + artifact.Path
		}
		if artifact.Name != "" {
			return artifact.OriginalDeploymentRepo + "/" + artifact.Name
		}
	}

	// Fallback: Use Path directly - it might be a complete path or might fail
	// If it fails, setPropsOnArtifacts will warn and move on
	if artifact.Path != "" {
		return artifact.Path
	}

	// Last resort: just the name (unlikely to work, but let it try)
	if artifact.Name != "" {
		return artifact.Name
	}

	// Nothing available
	return ""
}

// constructArtifactPath builds the full Artifactory path for an artifact (legacy function).
func constructArtifactPath(artifact buildinfo.Artifact) string {
	if artifact.OriginalDeploymentRepo == "" {
		return ""
	}
	if artifact.Path != "" {
		return artifact.OriginalDeploymentRepo + "/" + artifact.Path
	}
	if artifact.Name != "" {
		return artifact.OriginalDeploymentRepo + "/" + artifact.Name
	}
	return ""
}

// setPropsOnArtifacts sets properties on multiple artifacts using search-based resolution.
// This uses the same search mechanism as 'jf rt set-props', which resolves virtual repository
// paths to their underlying local repositories before setting properties.
// If property setting fails after retries, logs a warning and continues (does not fail the build).
func setPropsOnArtifacts(servicesManager artifactory.ArtifactoryServicesManager, artifactPaths []string, props string) {
	if len(artifactPaths) == 0 {
		return
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := retryDelayBase * time.Duration(1<<(attempt-1))
			log.Debug("Retrying property set for artifacts (attempt", attempt+1, "/", maxRetries, ") after", delay)
			time.Sleep(delay)
		}

		// Build spec from artifact paths - this enables virtual repo resolution
		specFiles := buildSpecFromPaths(artifactPaths)

		// Use SearchItems to resolve paths (including virtual -> local repo resolution)
		reader, err := generic.SearchItems(specFiles, servicesManager)
		if err != nil {
			log.Debug("CI VCS: Search failed for artifacts:", err)
			lastErr = err
			continue
		}

		// Check if any artifacts were found
		length, _ := reader.Length()
		if length == 0 {
			if closeErr := reader.Close(); closeErr != nil {
				log.Debug("Failed to close reader:", closeErr)
			}
			log.Debug("CI VCS: No artifacts found via search, paths may not exist")
			return
		}
		params := services.PropsParams{
			Reader:       reader,
			Props:        props,
			UseDebugLogs: true,
		}
		successCount, err := servicesManager.SetProps(params)
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close reader:", closeErr)
		}
		if err == nil {
			log.Debug("CI VCS: Successfully set properties on", successCount, "artifacts")
			return
		}

		// Check if error is 404 - artifact path might be incorrect, skip silently
		if is404Error(err) {
			log.Debug("CI VCS: SetProps returned 404 - some artifacts not found")
			return
		}
		// Check if error is 403 - permission issue, skip silently
		if is403Error(err) {
			if attempt >= 1 {
				log.Debug("CI VCS: SetProps returned 403 - permission denied")
				return
			}
		}
		lastErr = err
		log.Debug("CI VCS: Batch attempt", attempt+1, "failed:", err)
	}
	log.Debug("CI VCS: Failed to set properties after", maxRetries, "attempts:", lastErr)
}

// buildSpecFromPaths creates a SpecFiles object from artifact paths for search-based resolution.
// Each path becomes a separate file pattern in the spec.
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
