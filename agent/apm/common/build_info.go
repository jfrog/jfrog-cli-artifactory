package apmcommon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/crypto"
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

// ShouldCollectBuildInfo reports whether the user enabled build-info collection
// (--build-name/--build-number or JFROG_CLI_BUILD_*). Used by install/update/publish to avoid
// "skipping build-info" noise when collection was never requested.
func ShouldCollectBuildInfo(buildConfig *buildUtils.BuildConfiguration) (bool, error) {
	if buildConfig == nil {
		return false, nil
	}
	return buildConfig.IsCollectBuildInfo()
}

// CollectAndSaveInstallBuildInfo reads the lockfile, resolves checksums, and saves build-info.
// Runs only when build info collection is enabled.
func CollectAndSaveInstallBuildInfo(lockfilePath, manifestPath string, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
	collectBuildInfo, err := ShouldCollectBuildInfo(buildConfig)
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
		entityDep := dep.ToEntitiesDependency(checksum)
		entityDep.RequestedBy = anchorRequestedByToModule(entityDep.RequestedBy, moduleID)
		entityDeps = append(entityDeps, entityDep)
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

// anchorRequestedByToModule appends moduleID as the terminal element of every requestedBy chain,
// so each chain ends at the consuming module's id, matching npm/yarn/go/cargo's convention.
func anchorRequestedByToModule(requestedBy [][]string, moduleID string) [][]string {
	if len(requestedBy) == 0 {
		return [][]string{{moduleID}}
	}
	anchored := make([][]string, len(requestedBy))
	for i, chain := range requestedBy {
		anchored[i] = append(append([]string{}, chain...), moduleID)
	}
	return anchored
}

// buildArtifactPathParts derives (fileName, dirPath, artifactPath) for a published APM package
// from the assumed agentpackages storage convention: {repo}/{owner}/{packageName}/{packageName}-
// {version}.zip. This is a naming convention this code assumes, not something read back from
// Artifactory - verifyArtifactPathExists is how callers can confirm it against the real repo.
func buildArtifactPathParts(owner, packageName, version string) (fileName, dirPath, artifactPath string) {
	fileName = packageName + "-" + version + "." + apmPackageFileExtension
	dirPath = packageName
	artifactPath = fileName
	if owner != "" {
		dirPath = owner + "/" + packageName
		artifactPath = dirPath + "/" + fileName
	}
	return fileName, dirPath, artifactPath
}

// verifyArtifactPathExists issues an HTTP HEAD directly against repoName/artifactPath (the exact
// path build-info is about to record) to confirm apm's agentpackages storage convention actually
// matches where the file lives. Best-effort only: returns false and logs a warning on any
// failure, but never blocks or fails the publish - build-info is still recorded either way.
func verifyArtifactPathExists(serverDetails *config.ServerDetails, repoName, artifactPath string) bool {
	if serverDetails == nil || repoName == "" || artifactPath == "" {
		return false
	}
	servicesManager, err := artCoreUtils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("apm publish: could not create service manager for artifact path verification:", err.Error())
		return false
	}

	base := strings.TrimSuffix(serverDetails.GetArtifactoryUrl(), "/")
	directURL := base + "/" + repoName + "/" + artifactPath
	clientDetails := servicesManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	if _, _, err := servicesManager.Client().GetRemoteFileDetails(directURL, &clientDetails); err != nil {
		log.Warn(fmt.Sprintf(
			"apm publish: could not confirm the assumed artifact path %q exists in %s (%s); "+
				"build-info will still record this path, but it may not match where apm actually stored the file.",
			artifactPath, repoName, err.Error()))
		return false
	}
	return true
}

// SavePublishBuildInfo saves build artifact info for a published APM package. moduleName is the
// project's own identity (apm.yml's name:, for the build-info module id); packageName is the
// identity apm actually uploaded under via --package (owner/packageName), which Artifactory's
// agentpackages storage layout uses for the real path: {repo}/{owner}/{packageName}/{packageName}-{version}.zip.
// These two can differ, and only packageName reflects where the artifact actually lives.
func SavePublishBuildInfo(owner, moduleName, packageName, version string, checksum entities.Checksum, repoName string, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
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

	// Same convention as npm's BuildInfoModuleId(): "" unless BOTH name and version are set,
	// never a partial "name:" or ":version". An empty moduleID isn't special-cased further -
	// it flows to build-info-go's generic partial-merge fallback (module.Id = build name), same
	// as install/update's derivedModuleID and same as npm's own AddNpmModule.
	moduleID := buildConfig.GetModule()
	if moduleID == "" && moduleName != "" && version != "" {
		moduleID = moduleName + ":" + version
	}

	fileName, dirPath, artifactPath := buildArtifactPathParts(owner, packageName, version)

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

// CollectAndSavePublishBuildInfo reads the package name/version from apm.yml, resolves the
// just-published artifact's checksum, and records it in build-info. Runs only when build info
// collection is enabled.
//
// Checksum resolution has two tiers, same shape as install's cache-then-HEAD-then-lockfile chain:
//  1. HTTP HEAD against the artifact's own download URL (unchanged - still the primary source).
//  2. Fallback: hash the local zip apm just packed in the working directory. cargo and ruby both
//     use exactly this as their primary source for a published artifact's checksum
//     (crypto.GetFileDetails on the local .crate/.gem file) - apm previously had no fallback tier
//     at all here, unlike its own install-side resolution.
func CollectAndSavePublishBuildInfo(manifestPath, owner, packageName, repoName, explicitZipPath string, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) error {
	collectBuildInfo, err := ShouldCollectBuildInfo(buildConfig)
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
	// No skip-on-missing-name/version check here, matching npm's own publish path: npm never
	// special-cases an incomplete package.json before computing its module id/deploy path
	// either - an empty name/version just flows through to whatever moduleID/artifact path
	// that produces (see SavePublishBuildInfo's own moduleID fallback below).
	if packageName == "" {
		packageName = manifest.Name
	}

	// Best-effort confirmation that the assumed agentpackages path actually holds the file
	// build-info is about to record it under. Never blocks the publish either way.
	_, _, artifactPath := buildArtifactPathParts(owner, packageName, manifest.Version)
	verifyArtifactPathExists(serverDetails, repoName, artifactPath)

	checksum := lookupPublishedArtifactChecksum(owner, packageName, manifest.Version, repoName, serverDetails)
	if !hasAnyChecksum(checksum) {
		zipPath := explicitZipPath
		if zipPath == "" {
			// apm always packs the local zip from apm.yml's own name, regardless of --package.
			zipPath = manifest.Name + "-" + manifest.Version + "." + apmPackageFileExtension
		}
		if !filepath.IsAbs(zipPath) {
			zipPath = filepath.Join(filepath.Dir(manifestPath), zipPath)
		}
		if localChecksum, localErr := localPackedArtifactChecksum(zipPath); localErr == nil {
			log.Debug("apm publish: HEAD lookup returned no checksum; using the local packed zip's own hash instead.")
			checksum = localChecksum
		} else {
			log.Warn(fmt.Sprintf(
				"apm publish: could not resolve a checksum for %s@%s from Artifactory or the local packed zip (%s); "+
					"build-info will record this artifact with no checksum.",
				packageName, manifest.Version, localErr.Error()))
		}
	}
	return SavePublishBuildInfo(owner, manifest.Name, packageName, manifest.Version, checksum, repoName, serverDetails, buildConfig)
}

// lookupPublishedArtifactChecksum issues an HTTP HEAD against the just-published artifact's own
// download URL and reads its checksum from Artifactory's X-Checksum-* response headers. Returns
// an empty Checksum (not an error) if the repo/owner are unknown or the lookup fails - the caller
// falls back to hashing the local packed zip, so a missing checksum here isn't yet the final word.
func lookupPublishedArtifactChecksum(owner, packageName, version, repoName string, serverDetails *config.ServerDetails) entities.Checksum {
	if owner == "" || repoName == "" || serverDetails == nil {
		return entities.Checksum{}
	}
	servicesManager, err := artCoreUtils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		log.Warn("apm publish: could not create service manager for checksum lookup:", err.Error())
		return entities.Checksum{}
	}

	downloadURL := AgentPackagesBaseURL(serverDetails, repoName) + "v1/packages/" + owner + "/" + packageName + "/versions/" + version + "/download"
	clientDetails := servicesManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	fileDetails, _, err := servicesManager.Client().GetRemoteFileDetails(downloadURL, &clientDetails)
	if err != nil {
		log.Warn(fmt.Sprintf("apm publish: checksum HEAD lookup failed for %s: %s", downloadURL, err.Error()))
		return entities.Checksum{}
	}
	return fileDetails.Checksum
}

// localPackedArtifactChecksum hashes the zip apm just published from disk (the auto-packed
// {name}-{version}.zip, or whatever path --zip pointed at), mirroring cargo's and ruby's pattern
// of hashing the local artifact file directly (gofrog/crypto.GetFileDetails).
func localPackedArtifactChecksum(zipPath string) (entities.Checksum, error) {
	fileDetails, err := crypto.GetFileDetails(zipPath, true)
	if err != nil {
		return entities.Checksum{}, fmt.Errorf("hash local packed zip %s: %w", zipPath, err)
	}
	return entities.Checksum{
		Sha1:   fileDetails.Checksum.Sha1,
		Sha256: fileDetails.Checksum.Sha256,
		Md5:    fileDetails.Checksum.Md5,
	}, nil
}

// derivedModuleID returns the default module ID for the install-side build-info module: manifest
// name:version, matching how npm and yarn derive their module ID (packageInfo.BuildInfoModuleId(),
// always "name:version") for every module they create, install or publish alike. Falls back to
// matching npm/yarn's own BuildInfoModuleId() convention exactly - including returning "" (not
// a fallback of our own) when apm.yml can't be read or its name/version are empty. An empty
// module ID isn't a special case this package needs to handle: like npm's AddNpmModule, it flows
// through to build-info-go's generic partial-merge fallback (module.Id = build name), the same
// path every other package manager's empty module ID takes.
func derivedModuleID(manifestPath string) string {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		log.Debug("apm.yml parsing failed while deriving install module ID:", err.Error())
		return ""
	}
	if manifest.Name == "" || manifest.Version == "" {
		return ""
	}
	return manifest.Name + ":" + manifest.Version
}
