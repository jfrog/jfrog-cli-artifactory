package flexpack

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func searchRecentArtifacts(servicesManager artifactory.ArtifactoryServicesManager, buildInfo *entities.BuildInfo, startTime time.Time, workingDir string) ([]specutils.ResultItem, error) {
	var recentArtifacts []specutils.ResultItem
	// The repository typically depends on whether the version is a snapshot or release.
	repoCache := make(map[bool]string)

	for _, module := range buildInfo.Modules {
		parts := strings.Split(module.Id, ":")
		if len(parts) < 3 {
			log.Warn("Skipping module with invalid ID format: " + module.Id)
			continue
		}
		version := parts[2]
		isSnapshot := strings.Contains(strings.ToLower(version), keywordSnapshot)

		targetRepo, ok := repoCache[isSnapshot]
		if !ok {
			moduleWorkingDir := workingDir
			if props, ok := module.Properties.(map[string]string); ok {
				if subPath, exists := props["module_path"]; exists {
					moduleWorkingDir = filepath.Join(workingDir, subPath)
				}
			}

			var deployErr error
			targetRepo, deployErr = getGradleDeployRepository(moduleWorkingDir, workingDir, version)
			if deployErr != nil && moduleWorkingDir != workingDir {
				log.Debug(fmt.Sprintf("Repo not found in module dir %s, trying root: %v", moduleWorkingDir, deployErr))
				targetRepo, deployErr = getGradleDeployRepository(workingDir, workingDir, version)
			}

			if deployErr == nil {
				repoCache[isSnapshot] = targetRepo
			} else if len(module.Artifacts) > 0 {
				log.Warn(fmt.Sprintf("Could not determine Gradle deploy repository for module %s: %v", module.Id, deployErr))
				continue
			}
		}

		if len(module.Artifacts) == 0 {
			continue
		}
		// We assume all artifacts in a module go to the same repo structure
		artifact := module.Artifacts[0]
		var artifactPath string
		if artifact.Path != "" {
			artifactPath = fmt.Sprintf("%s/%s", targetRepo, artifact.Path)
		} else {
			groupId := parts[0]
			artifactId := parts[1]
			groupPath := strings.ReplaceAll(groupId, ".", "/")
			if groupId == "unspecified" {
				groupPath = "**"
			}
			versionPath := version
			if version == "unspecified" {
				versionPath = "*"
			}
			artifactPath = fmt.Sprintf("%s/%s/%s/%s/%s-*",
				targetRepo,
				groupPath, artifactId, versionPath, artifactId)
		}

		// Let's use the directory of the artifact to find all related artifacts (jars, poms, etc)
		artifactDir := path.Dir(artifactPath)
		searchPattern := fmt.Sprintf("%s/*", artifactDir)

		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Pattern: searchPattern,
			},
		}
		searchReader, err := servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to search for deployed artifacts for module %s: %v", module.Id, err))
			continue
		}

		// Filter to only artifacts modified after the build started
		for item := new(specutils.ResultItem); searchReader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
			modTime, err := parseArtifactModifiedTime(item.Modified)
			if err != nil {
				log.Debug("Could not parse modified time for " + item.Name + ": " + err.Error())
				continue
			}
			if modTime.After(startTime.Add(-artifactSearchClockSkewBuffer)) {
				recentArtifacts = append(recentArtifacts, *item)
			}
		}
		if closeErr := searchReader.Close(); closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close search reader: %s", closeErr))
		}
	}
	return recentArtifacts, nil
}

func parseArtifactModifiedTime(modified string) (time.Time, error) {
	formats := []string{
		time.RFC3339Nano,                // 2006-01-02T15:04:05.999999999Z07:00
		time.RFC3339,                    // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05.999Z",      // ISO 8601 with milliseconds and Z
		"2006-01-02T15:04:05.000-0700",  // Build info format
		"2006-01-02T15:04:05.999-07:00", // ISO 8601 with milliseconds and timezone
	}

	for _, format := range formats {
		if t, err := time.Parse(format, modified); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %s", modified)
}
