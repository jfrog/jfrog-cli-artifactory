package buildinfo

import (
	"path"
	"strings"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	artclientutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
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

// setPropsOnArtifacts sets properties on multiple artifacts in a single API call with retry logic.
// This is a major performance optimization over setting properties one by one.
// If property setting fails after retries, logs a warning and continues (does not fail the build).
func setPropsOnArtifacts(
	servicesManager artifactory.ArtifactoryServicesManager,
	artifactPaths []string,
	props string,
) {
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

		// Create reader for all artifacts
		reader, err := createArtifactsReader(artifactPaths)
		if err != nil {
			log.Debug("Failed to create reader for CI VCS properties:", err)
			return
		}

		params := services.PropsParams{
			Reader: reader,
			Props:  props,
		}

		successCount, err := servicesManager.SetProps(params)
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close reader:", closeErr)
		}

		if err == nil {
			log.Info("CI VCS: Successfully set properties on", successCount, "artifacts")
			return
		}

		// Check if error is 404 - artifact path might be incorrect, skip silently
		if is404Error(err) {
			log.Info("CI VCS: SetProps returned 404 - some artifacts not found (path may be incomplete)")
			return
		}

		// Check if error is 403 - permission issue, skip silently
		if is403Error(err) {
			if attempt >= 1 {
				log.Info("CI VCS: SetProps returned 403 - permission denied")
				return
			}
		}

		lastErr = err
		log.Info("CI VCS: Batch attempt", attempt+1, "failed:", err)
	}

	log.Info("CI VCS: Failed to set properties after", maxRetries, "attempts:", lastErr)
}

// createArtifactsReader creates a ContentReader containing all artifact paths for batch processing.
func createArtifactsReader(artifactPaths []string) (*content.ContentReader, error) {
	writer, err := content.NewContentWriter("results", true, false)
	if err != nil {
		return nil, err
	}

	for _, artifactPath := range artifactPaths {
		// Parse path into repo/path/name
		parts := strings.SplitN(artifactPath, "/", 2)
		if len(parts) < 2 {
			log.Debug("Invalid artifact path skipped during reader creation:", artifactPath)
			continue
		}

		repo := parts[0]
		pathAndName := parts[1]
		dir, name := path.Split(pathAndName)

		writer.Write(artclientutils.ResultItem{
			Repo: repo,
			Path: strings.TrimSuffix(dir, "/"),
			Name: name,
			Type: "file",
		})
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return content.NewContentReader(writer.GetFilePath(), "results"), nil
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
