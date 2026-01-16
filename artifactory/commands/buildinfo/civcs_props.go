package buildinfo

import (
	"fmt"
	"path"
	"strings"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	artclientutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// propsResult represents the outcome of setting properties on an artifact.
type propsResult int

const (
	propsResultSuccess propsResult = iota
	propsResultNotFound
	propsResultFailed
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

// buildCIVcsPropsString constructs the properties string from CI VCS info.
func buildCIVcsPropsString(info cienv.CIVcsInfo) string {
	var parts []string
	if info.Provider != "" {
		parts = append(parts, "vcs.provider="+info.Provider)
	}
	if info.Org != "" {
		parts = append(parts, "vcs.org="+info.Org)
	}
	if info.Repo != "" {
		parts = append(parts, "vcs.repo="+info.Repo)
	}
	return strings.Join(parts, ";")
}

// setPropsWithRetry sets properties on an artifact with retry logic.
// Returns:
// - propsResultSuccess: properties set successfully
// - propsResultNotFound: artifact not found (404)
// - propsResultFailed: failed after all retries
func setPropsWithRetry(
	servicesManager artifactory.ArtifactoryServicesManager,
	artifactPath string,
	props string,
) propsResult {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := retryDelayBase * time.Duration(1<<(attempt-1))
			log.Debug("Retrying property set for", artifactPath, "(attempt", attempt+1, "/", maxRetries, ") after", delay)
			time.Sleep(delay)
		}

		// Create reader for single artifact
		reader, err := createSingleArtifactReader(artifactPath)
		if err != nil {
			log.Debug("Failed to create reader for", artifactPath, ":", err)
			return propsResultFailed
		}
		params := services.PropsParams{
			Reader: reader,
			Props:  props,
		}
		_, err = servicesManager.SetProps(params)
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("Failed to close reader for", artifactPath, ":", closeErr)
		}
		if err == nil {
			log.Debug("Set CI VCS properties on:", artifactPath)
			return propsResultSuccess
		}

		// Check if error is 404 - don't retry
		if is404Error(err) {
			return propsResultNotFound
		}

		// Check if error is 403 - limited retries
		if is403Error(err) {
			// 403 might be temporary (token refresh) or permanent (no permission)
			// Retry once, then give up
			if attempt >= 1 {
				log.Debug("403 Forbidden persists, likely permission issue")
				return propsResultFailed
			}
		}
		lastErr = err
		log.Debug("Attempt", attempt+1, "failed for", artifactPath, ":", err)
	}

	// All retries exhausted
	log.Debug("All", maxRetries, "attempts failed for", artifactPath, ":", lastErr)
	return propsResultFailed
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

// createSingleArtifactReader creates a ContentReader for a single artifact path.
func createSingleArtifactReader(artifactPath string) (*content.ContentReader, error) {
	writer, err := content.NewContentWriter("results", true, false)
	if err != nil {
		return nil, err
	}

	// Parse path into repo/path/name
	parts := strings.SplitN(artifactPath, "/", 2)
	if len(parts) < 2 {
		if closeErr := writer.Close(); closeErr != nil {
			log.Debug("Failed to close writer:", closeErr)
		}
		return nil, fmt.Errorf("invalid artifact path: %s", artifactPath)
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
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return content.NewContentReader(writer.GetFilePath(), "results"), nil
}
