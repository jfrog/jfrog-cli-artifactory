package buildinfo

import (
	"fmt"
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

// extractArtifactPathsWithWarnings extracts full Artifactory paths from build info artifacts.
// Returns the list of valid paths and count of skipped artifacts (missing repo path).
// Logs a warning for each artifact missing OriginalDeploymentRepo.
func extractArtifactPathsWithWarnings(buildInfo *buildinfo.BuildInfo) ([]string, int) {
	var paths []string
	var skippedCount int

	for _, module := range buildInfo.Modules {
		for _, artifact := range module.Artifacts {
			if artifact.OriginalDeploymentRepo == "" {
				// OriginalDeploymentRepo is required to construct the full Artifactory path
				// (e.g., "libs-release-local/com/example/artifact.jar") for setting properties.
				// Without the repository name, we cannot target the artifact in Artifactory API.
				artifactIdentifier := artifact.Name
				if artifactIdentifier == "" {
					artifactIdentifier = artifact.Path
				}
				if artifactIdentifier == "" {
					artifactIdentifier = fmt.Sprintf("sha1:%s", artifact.Sha1)
				}
				log.Warn("Unable to find repo path for artifact:", artifactIdentifier)
				skippedCount++
				continue
			}
			fullPath := constructArtifactPath(artifact)
			if fullPath != "" {
				paths = append(paths, fullPath)
			}
		}
	}
	return paths, skippedCount
}

// constructArtifactPath builds the full Artifactory path for an artifact.
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
			log.Debug("Failed to create reader for artifacts:", err)
			return
		}

		params := services.PropsParams{
			Reader: reader,
			Props:  props,
		}

		_, err = servicesManager.SetProps(params)
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close reader:", closeErr)
		}

		if err == nil {
			log.Debug("Successfully set CI VCS properties on artifacts")
			return
		}

		// Check if error is 404 - don't retry (some items might not exist, but we can't tell which ones from a batch call easily)
		if is404Error(err) {
			log.Debug("Batch property set returned 404, some artifacts might not exist")
			return
		}

		// Check if error is 403 - limited retries
		if is403Error(err) {
			if attempt >= 1 {
				log.Debug("403 Forbidden persists, likely permission issue")
				return
			}
		}

		lastErr = err
		log.Debug("Batch attempt", attempt+1, "failed:", err)
	}

	log.Debug("All", maxRetries, "batch attempts failed:", lastErr)
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
