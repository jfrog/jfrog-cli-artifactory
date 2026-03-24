// Package conan provides Conan package manager integration for JFrog Artifactory.
package conan

import (
	"fmt"

	"github.com/jfrog/build-info-go/entities"
	conanflex "github.com/jfrog/build-info-go/flexpack/conan"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ConanUploadOutput represents the JSON output of 'conan upload --format=json'.
// The structure is a Conan PackageList keyed by remote name:
//
//	{
//	  "remote-name": {
//	    "pkg/1.0": {
//	      "revisions": {
//	        "recipe_rev_hash": {
//	          "timestamp": 1667396813.987,
//	          "packages": {
//	            "package_id_hash": {
//	              "revisions": {
//	                "pkg_rev_hash": {}
//	              }
//	            }
//	          }
//	        }
//	      }
//	    }
//	  }
//	}
type ConanUploadOutput map[string]map[string]ConanUploadRecipe

// ConanUploadRecipe represents a recipe entry in the upload output.
type ConanUploadRecipe struct {
	Revisions map[string]ConanUploadRecipeRevision `json:"revisions"`
}

// ConanUploadRecipeRevision represents a recipe revision with its packages.
type ConanUploadRecipeRevision struct {
	Timestamp float64                            `json:"timestamp"`
	Packages  map[string]ConanUploadPackageEntry `json:"packages"`
}

// ConanUploadPackageEntry represents a binary package entry.
type ConanUploadPackageEntry struct {
	Revisions map[string]interface{}  `json:"revisions"`
	Info      *ConanUploadPackageInfo `json:"info"`
}

// ConanUploadPackageInfo contains package metadata like settings and options.
type ConanUploadPackageInfo struct {
	Settings map[string]string `json:"settings"`
	Options  map[string]string `json:"options"`
}

// UploadProcessor processes structured Conan upload output and collects build info.
// It uses the JSON PackageList from 'conan upload --format=json' to:
// 1. Determine which artifacts were uploaded (avoiding collection of all revisions)
// 2. Collect dependencies from the local project
// 3. Set build properties on uploaded artifacts
type UploadProcessor struct {
	workingDir         string
	buildConfiguration *buildUtils.BuildConfiguration
	serverDetails      *config.ServerDetails
	conanConfig        conanflex.ConanConfig
}

// NewUploadProcessor creates a new upload processor.
func NewUploadProcessor(workingDir string, buildConfig *buildUtils.BuildConfiguration, serverDetails *config.ServerDetails, conanConfig conanflex.ConanConfig) *UploadProcessor {
	return &UploadProcessor{
		workingDir:         workingDir,
		buildConfiguration: buildConfig,
		serverDetails:      serverDetails,
		conanConfig:        conanConfig,
	}
}

// ProcessJSON processes the structured JSON upload output and collects build info.
func (up *UploadProcessor) ProcessJSON(uploadOutput ConanUploadOutput) error {
	remoteName, packages := up.extractUploadInfo(uploadOutput)
	if remoteName == "" || len(packages) == 0 {
		log.Debug("No upload data found in JSON output")
		return nil
	}

	var packageRef string
	for ref := range packages {
		packageRef = ref
		break
	}
	log.Debug(fmt.Sprintf("Processing upload for package: %s to remote: %s", packageRef, remoteName))

	uploadedPaths := up.buildArtifactPathsFromJSON(packages)
	log.Debug(fmt.Sprintf("Found %d uploaded artifact paths", len(uploadedPaths)))

	buildInfo, err := up.collectDependencies()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to collect dependencies: %v", err))
		buildInfo = up.createEmptyBuildInfo(packageRef)
	}

	targetRepo, err := up.getTargetRepositoryFromRemote(remoteName)
	if err != nil {
		log.Warn(fmt.Sprintf("Could not determine target repository: %v", err))
		log.Warn("Build info will be saved but artifacts may not be linked correctly")
		return up.saveBuildInfo(buildInfo)
	}
	log.Debug(fmt.Sprintf("Using Artifactory repository: %s", targetRepo))

	if up.serverDetails != nil && len(uploadedPaths) > 0 {
		artifacts, collectErr := up.collectUploadedArtifacts(uploadedPaths, targetRepo)
		if collectErr != nil {
			log.Warn(fmt.Sprintf("Failed to collect artifacts: %v", collectErr))
		} else {
			up.addArtifactsToModule(buildInfo, artifacts)

			if len(artifacts) > 0 {
				if err := up.setBuildProperties(artifacts, targetRepo); err != nil {
					log.Warn(fmt.Sprintf("Failed to set build properties: %v", err))
				}
			}
		}
	}

	return up.saveBuildInfo(buildInfo)
}

// extractUploadInfo extracts the remote name and package map from the upload output.
// Conan upload targets a single remote, so typically only one entry exists.
// If multiple remotes are present, the first one with packages is used.
func (up *UploadProcessor) extractUploadInfo(output ConanUploadOutput) (string, map[string]ConanUploadRecipe) {
	if len(output) > 1 {
		log.Debug(fmt.Sprintf("Upload output contains %d remotes, processing only the first with packages", len(output)))
	}
	for remoteName, packages := range output {
		if len(packages) > 0 {
			return remoteName, packages
		}
	}
	return "", nil
}

// buildArtifactPathsFromJSON builds Artifactory paths from the structured upload JSON.
// Paths follow Conan's Artifactory layout:
//   - Recipe: _/{name}/{version}/_/{recipe_rev}/export
//   - Package: _/{name}/{version}/_/{recipe_rev}/package/{pkg_id}/{pkg_rev}
func (up *UploadProcessor) buildArtifactPathsFromJSON(packages map[string]ConanUploadRecipe) []string {
	var paths []string
	for pkgRef, recipe := range packages {
		for recipeRev, revData := range recipe.Revisions {
			recipePath := fmt.Sprintf("_/%s/_/%s/export", pkgRef, recipeRev)
			paths = append(paths, recipePath)

			for pkgID, pkgEntry := range revData.Packages {
				for pkgRev := range pkgEntry.Revisions {
					pkgPath := fmt.Sprintf("_/%s/_/%s/package/%s/%s", pkgRef, recipeRev, pkgID, pkgRev)
					paths = append(paths, pkgPath)
				}
			}
		}
	}
	return paths
}

// getTargetRepositoryFromRemote resolves a Conan remote name to an Artifactory repository name.
func (up *UploadProcessor) getTargetRepositoryFromRemote(remoteName string) (string, error) {
	remoteURL, err := getRemoteURL(remoteName)
	if err != nil {
		return "", fmt.Errorf("could not get URL for remote '%s': %w", remoteName, err)
	}

	if !isArtifactoryConanRemote(remoteURL) {
		return "", fmt.Errorf("remote '%s' is not an Artifactory Conan repository (URL: %s)", remoteName, remoteURL)
	}

	repoName := ExtractRepoName(remoteURL)
	if repoName == "" {
		return "", fmt.Errorf("could not extract repository name from URL: %s", remoteURL)
	}

	return repoName, nil
}

// collectUploadedArtifacts collects only artifacts from specific paths that were uploaded.
func (up *UploadProcessor) collectUploadedArtifacts(uploadedPaths []string, targetRepo string) ([]entities.Artifact, error) {
	if up.serverDetails == nil {
		return nil, fmt.Errorf("server details not initialized")
	}

	collector := NewArtifactCollector(up.serverDetails, targetRepo)
	var allArtifacts []entities.Artifact

	for _, path := range uploadedPaths {
		artifacts, err := collector.CollectArtifactsForPath(path)
		if err != nil {
			log.Debug(fmt.Sprintf("Failed to collect artifacts for path %s: %v", path, err))
			continue
		}
		allArtifacts = append(allArtifacts, artifacts...)
	}

	log.Info(fmt.Sprintf("Collected %d Conan artifacts", len(allArtifacts)))
	return allArtifacts, nil
}

// collectDependencies collects dependencies using FlexPack.
func (up *UploadProcessor) collectDependencies() (*entities.BuildInfo, error) {
	buildName, err := up.buildConfiguration.GetBuildName()
	if err != nil {
		return nil, fmt.Errorf("get build name: %w", err)
	}

	buildNumber, err := up.buildConfiguration.GetBuildNumber()
	if err != nil {
		return nil, fmt.Errorf("get build number: %w", err)
	}

	collector, err := conanflex.NewConanFlexPack(up.conanConfig)
	if err != nil {
		return nil, fmt.Errorf("create conan flexpack: %w", err)
	}

	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return nil, fmt.Errorf("collect build info: %w", err)
	}

	log.Debug(fmt.Sprintf("Collected build info with %d modules", len(buildInfo.Modules)))
	return buildInfo, nil
}

// createEmptyBuildInfo creates a minimal build info when dependency collection fails.
func (up *UploadProcessor) createEmptyBuildInfo(packageRef string) *entities.BuildInfo {
	buildName, _ := up.buildConfiguration.GetBuildName()
	buildNumber, _ := up.buildConfiguration.GetBuildNumber()

	return &entities.BuildInfo{
		Name:    buildName,
		Number:  buildNumber,
		Modules: []entities.Module{{Id: packageRef, Type: entities.Conan}},
	}
}

// addArtifactsToModule adds artifacts to the first module in build info.
func (up *UploadProcessor) addArtifactsToModule(buildInfo *entities.BuildInfo, artifacts []entities.Artifact) {
	if len(buildInfo.Modules) == 0 {
		return
	}
	buildInfo.Modules[0].Artifacts = artifacts
}

// setBuildProperties sets build properties on artifacts in Artifactory.
func (up *UploadProcessor) setBuildProperties(artifacts []entities.Artifact, targetRepo string) error {
	buildName, err := up.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}

	buildNumber, err := up.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	projectKey := up.buildConfiguration.GetProject()

	setter := NewBuildPropertySetter(up.serverDetails, targetRepo, buildName, buildNumber, projectKey)
	return setter.SetProperties(artifacts)
}

// saveBuildInfo saves the build info for later publishing.
func (up *UploadProcessor) saveBuildInfo(buildInfo *entities.BuildInfo) error {
	service := buildUtils.CreateBuildInfoService()

	projectKey := up.buildConfiguration.GetProject()
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}

	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}

	log.Info("Conan build info saved locally")
	return nil
}
