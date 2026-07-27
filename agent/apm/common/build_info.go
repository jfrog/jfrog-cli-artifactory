package apmcommon

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jfrog/build-info-go/entities"
	artUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	artCoreUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// CollectAndSaveInstallBuildInfo reads the lockfile, resolves checksums, and saves build-info.
// Runs only when build info collection is enabled.
func CollectAndSaveInstallBuildInfo(lockfilePath, manifestPath string, sd *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
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
			// apm doesn't write apm.lock.yaml at all when a project has zero dependencies
			// ("No changes -- install state already up to date") - this is the expected,
			// common case, not a failure.
			log.Info("No apm.lock.yaml found (project has no dependencies). Skipping build info.")
			return nil
		}
		return err
	}
	if len(deps) == 0 {
		log.Info("No registry dependencies found in lockfile. Skipping build info.")
		return nil
	}

	checksumMap, err := ResolveChecksums(deps, sd, buildConfig)
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
		return errorutils.CheckErrorf("build info collection is not enabled")
	}

	moduleID := buildConfig.GetModule()
	if moduleID == "" {
		moduleID = derivedModuleID(manifestPath)
	}

	entityDeps := make([]entities.Dependency, 0, len(deps))
	for _, dep := range deps {
		cs := checksumMap[dep.ID]
		entityDeps = append(entityDeps, dep.ToEntitiesDependency(cs))
	}

	partial := &entities.Partial{
		ModuleId:     moduleID,
		ModuleType:   "apm",
		Dependencies: entityDeps,
	}
	if err = apmBuild.SavePartialBuildInfo(partial); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("APM build info saved for %s/%s: %d dependencies.", buildName, buildNumber, len(entityDeps)))
	return nil
}

// SavePublishBuildInfo saves build artifact info for a published APM package.
// Path/Name match Artifactory's real agentpackages storage layout, confirmed live:
// {repo}/{owner}/{name}/{name}-{version}.zip
func SavePublishBuildInfo(owner, name, version string, checksum entities.Checksum, repoName string, buildConfig *buildUtils.BuildConfiguration) error {
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
		return errorutils.CheckErrorf("build info collection is not enabled")
	}

	moduleID := buildConfig.GetModule()
	if moduleID == "" {
		moduleID = name + ":" + version
	}

	fileName := name + "-" + version + ".zip"
	artifactPath := fileName
	if owner != "" {
		artifactPath = owner + "/" + name + "/" + fileName
	}

	artifact := entities.Artifact{
		Name:                   fileName,
		Type:                   "zip",
		Path:                   artifactPath,
		OriginalDeploymentRepo: repoName,
		Checksum:               checksum,
	}

	if err = apmBuild.AddArtifacts(moduleID, "apm", artifact); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("APM publish build info saved for %s/%s.", buildName, buildNumber))
	return nil
}

// CollectAndSavePublishBuildInfo reads the package name/version from apm.yml, looks up the
// real checksum of the just-published artifact via AQL, and records it in build-info.
// Runs only when build info collection is enabled.
func CollectAndSavePublishBuildInfo(manifestPath, owner, repoName string, sd *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
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

	checksum := lookupPublishedArtifactChecksum(owner, manifest.Name, manifest.Version, repoName, sd)
	return SavePublishBuildInfo(owner, manifest.Name, manifest.Version, checksum, repoName, buildConfig)
}

// lookupPublishedArtifactChecksum queries AQL for the artifact apm publish just uploaded, by its
// known repo-relative path — confirmed live that AQL indexes agentpackages repos correctly.
// Returns an empty Checksum (not an error) if the repo/owner are unknown or the lookup fails,
// since a missing checksum shouldn't fail an already-successful publish.
func lookupPublishedArtifactChecksum(owner, name, version, repoName string, sd *config.ServerDetails) entities.Checksum {
	if owner == "" || repoName == "" || sd == nil {
		return entities.Checksum{}
	}
	servicesManager, err := artCoreUtils.CreateServiceManager(sd, -1, 0, false)
	if err != nil {
		log.Debug("apm publish: could not create service manager for checksum lookup:", err.Error())
		return entities.Checksum{}
	}

	dirPath := owner + "/" + name
	fileName := name + "-" + version + ".zip"
	query := fmt.Sprintf(
		`items.find({"repo":"%s","path":"%s","name":"%s"}).include("actual_sha1","sha256","actual_md5")`,
		repoName, dirPath, fileName)

	results, err := artUtils.ExecuteAqlQuery(servicesManager, query)
	if err != nil {
		log.Debug("apm publish: checksum AQL lookup failed:", err.Error())
		return entities.Checksum{}
	}
	if len(results) == 0 {
		log.Debug(fmt.Sprintf("apm publish: no AQL result for %s/%s — checksum will be empty", dirPath, fileName))
		return entities.Checksum{}
	}
	return entities.Checksum{Sha1: results[0].Actual_Sha1, Sha256: results[0].Sha256, Md5: results[0].Actual_Md5}
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
