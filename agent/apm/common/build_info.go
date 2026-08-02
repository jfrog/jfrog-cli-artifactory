package apmcommon

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jfrog/build-info-go/entities"
	artCliUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	artCoreUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// apmModuleType is the build-info module type for every apm-produced module (dependencies and
// published artifacts alike).
const apmModuleType = "apm"

// apmPackageFileExtension is the file type apm dependencies and published artifacts are stored
// as in Artifactory ({repo}/{owner}/{name}/{name}-{version}.zip), shared with
// dependency_resolver.go's ToEntitiesDependency.
const apmPackageFileExtension = "zip"

// errBuildInfoNotEnabled is returned by both saveInstallBuildInfo and SavePublishBuildInfo when
// buildUtils.PrepareBuildPrerequisites reports build-info collection isn't enabled.
const errBuildInfoNotEnabled = "build info collection is not enabled"

// CollectAndSaveInstallBuildInfo reads the lockfile, resolves checksums, and saves build-info.
// Runs only when build info collection is enabled.
func CollectAndSaveInstallBuildInfo(lockfilePath, manifestPath string, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
	collectBuildInfo, err := buildConfig.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if !collectBuildInfo {
		return nil
	}
	log.Info("Collecting APM build info...")

	deps, err := ResolveDependencies(lockfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// apm skips writing apm.lock.yaml when a project has zero dependencies - expected,
			// not a failure.
			log.Info("No apm.lock.yaml found (project has no dependencies). Skipping build info.")
			return nil
		}
		return err
	}
	if len(deps) == 0 {
		log.Info("No registry dependencies found in lockfile. Skipping build info.")
		return nil
	}

	checksumMap, err := ResolveChecksums(deps, serverDetails, buildConfig)
	if err != nil {
		return err
	}

	return saveInstallBuildInfo(deps, checksumMap, manifestPath, buildConfig)
}

func saveInstallBuildInfo(deps []ResolvedDep, checksumMap map[string]entities.Checksum, manifestPath string, buildConfig *buildUtils.BuildConfiguration) error {
	buildName, err := buildConfig.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := buildConfig.GetBuildNumber()
	if err != nil {
		return err
	}

	apmBuild, err := buildUtils.PrepareBuildPrerequisites(buildConfig)
	if err != nil {
		return err
	}
	if apmBuild == nil {
		return errorutils.CheckErrorf(errBuildInfoNotEnabled)
	}

	moduleID := buildConfig.GetModule()
	if moduleID == "" {
		moduleID = derivedModuleID(manifestPath)
	}

	entityDeps := make([]entities.Dependency, 0, len(deps))
	for _, dep := range deps {
		checksum := checksumMap[dep.ID]
		entityDeps = append(entityDeps, dep.ToEntitiesDependency(checksum))
	}

	partial := &entities.Partial{
		ModuleId:     moduleID,
		ModuleType:   apmModuleType,
		Dependencies: entityDeps,
	}
	if err = apmBuild.SavePartialBuildInfo(partial); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("APM build info saved for %s/%s: %d dependencies.", buildName, buildNumber, len(entityDeps)))
	return nil
}

// SavePublishBuildInfo saves build artifact info for a published APM package. Path/Name match
// Artifactory's agentpackages storage layout: {repo}/{owner}/{name}/{name}-{version}.zip
func SavePublishBuildInfo(owner, name, version string, checksum entities.Checksum, repoName string, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
	buildName, err := buildConfig.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := buildConfig.GetBuildNumber()
	if err != nil {
		return err
	}

	apmBuild, err := buildUtils.PrepareBuildPrerequisites(buildConfig)
	if err != nil {
		return err
	}
	if apmBuild == nil {
		return errorutils.CheckErrorf(errBuildInfoNotEnabled)
	}

	moduleID := buildConfig.GetModule()
	if moduleID == "" {
		moduleID = name + ":" + version
	}

	fileName := name + "-" + version + "." + apmPackageFileExtension
	dirPath := name
	artifactPath := fileName
	if owner != "" {
		dirPath = owner + "/" + name
		artifactPath = dirPath + "/" + fileName
	}

	artifact := entities.Artifact{
		Name:                   fileName,
		Type:                   apmPackageFileExtension,
		Path:                   artifactPath,
		OriginalDeploymentRepo: repoName,
		Checksum:               checksum,
	}

	if err = apmBuild.AddArtifacts(moduleID, apmModuleType, artifact); err != nil {
		return err
	}

	tagPublishedArtifactProperties(serverDetails, repoName, dirPath, fileName, buildConfig)

	log.Info(fmt.Sprintf("APM publish build info saved for %s/%s.", buildName, buildNumber))
	return nil
}

// tagPublishedArtifactProperties sets build.name/build.number/build.timestamp properties on the
// just-published artifact. Artifactory's build browser resolves an artifact's repo/path via a
// node_props join on these properties, not on checksum alone - without them it reports "No path
// found (externally resolved or deleted/overwritten)" even though the file exists. Every other
// publish-capable package manager in this repo (pnpm, npm, docker, conan, helm, etc.) already
// does this after upload; apm's publish flow was missing it. Best-effort: a failure here is
// logged but doesn't fail an already-successful publish.
func tagPublishedArtifactProperties(serverDetails *config.ServerDetails, repoName, dirPath, fileName string, buildConfig *buildUtils.BuildConfiguration) {
	if serverDetails == nil || repoName == "" {
		log.Debug("apm publish: skipping property tagging (no server details or repo name)")
		return
	}
	props, err := buildUtils.CreateBuildPropsFromConfiguration(buildConfig)
	if err != nil {
		log.Warn("apm publish: unable to create build properties:", err.Error())
		return
	}
	if props == "" {
		log.Debug("apm publish: no build properties to set (build collection disabled?)")
		return
	}
	log.Info(fmt.Sprintf("apm publish: setting build properties on %s/%s/%s", repoName, dirPath, fileName))

	servicesManager, err := artCoreUtils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		log.Warn("apm publish: unable to create service manager for property tagging:", err.Error())
		return
	}

	item := specutils.ResultItem{Repo: repoName, Path: dirPath, Name: fileName}
	pathToFile, err := artCliUtils.WriteResultItemsToFile([]specutils.ResultItem{item})
	if err != nil {
		log.Warn("apm publish: unable to write result items for property tagging:", err.Error())
		return
	}
	defer func() {
		if rmErr := os.Remove(pathToFile); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Debug("apm publish: failed to clean up result items file:", rmErr.Error())
		}
	}()

	reader := content.NewContentReader(pathToFile, content.DefaultKey)
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug("apm publish: failed to close result items reader:", closeErr.Error())
		}
	}()

	if _, err = servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true}); err != nil {
		log.Warn("apm publish: unable to set properties on published artifact:", err.Error(),
			"\nThis may cause the build to not properly link with the artifact. You can add properties manually.")
		return
	}
	log.Debug("apm publish: build properties set on published artifact.")
}

// CollectAndSavePublishBuildInfo reads the package name/version from apm.yml, looks up the
// real checksum of the just-published artifact via an HTTP HEAD, and records it in build-info.
// Runs only when build info collection is enabled.
func CollectAndSavePublishBuildInfo(manifestPath, owner, repoName string, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
	collectBuildInfo, err := buildConfig.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if !collectBuildInfo {
		return nil
	}
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	if manifest.Name == "" || manifest.Version == "" {
		log.Debug("APM manifest missing name or version; skipping publish build-info.")
		return nil
	}

	checksum := lookupPublishedArtifactChecksum(owner, manifest.Name, manifest.Version, repoName, serverDetails)
	return SavePublishBuildInfo(owner, manifest.Name, manifest.Version, checksum, repoName, serverDetails, buildConfig)
}

// lookupPublishedArtifactChecksum issues an HTTP HEAD against the just-published artifact's own
// download URL and reads its checksum from Artifactory's X-Checksum-* response headers. Returns
// an empty Checksum (not an error) if the repo/owner are unknown or the lookup fails, since a
// missing checksum shouldn't fail an already-successful publish.
func lookupPublishedArtifactChecksum(owner, name, version, repoName string, serverDetails *config.ServerDetails) entities.Checksum {
	if owner == "" || repoName == "" || serverDetails == nil {
		return entities.Checksum{}
	}
	servicesManager, err := artCoreUtils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("apm publish: could not create service manager for checksum lookup:", err.Error())
		return entities.Checksum{}
	}

	downloadURL := AgentPackagesBaseURL(serverDetails, repoName) + "v1/packages/" + owner + "/" + name + "/versions/" + version + "/download"
	clientDetails := servicesManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	fileDetails, _, err := servicesManager.Client().GetRemoteFileDetails(downloadURL, &clientDetails)
	if err != nil {
		log.Debug(fmt.Sprintf("apm publish: checksum HEAD lookup failed for %s: %s", downloadURL, err.Error()))
		return entities.Checksum{}
	}
	return fileDetails.Checksum
}

func derivedModuleID(manifestPath string) string {
	// Use directory name as module ID
	dir := filepath.Dir(manifestPath)
	base := filepath.Base(dir)
	if base == "." || base == "" {
		return "apm-project"
	}
	return base
}
