package flexpack

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// CollectMavenBuildInfoWithFlexPack collects Maven build info using FlexPack.
// userArgs is the goal/flag list the user invoked Maven with; its resolution flags (-P/-s/-D) are
// forwarded to FlexPack so dependency resolution matches the build. This follows the same pattern as
// Poetry FlexPack in poetry.go.
func CollectMavenBuildInfoWithFlexPack(workingDir, buildName, buildNumber string, buildConfiguration *buildUtils.BuildConfiguration, userArgs []string, serverDetails *config.ServerDetails) error {
	// Create Maven FlexPack configuration (following Poetry pattern)
	config := flexpack.MavenConfig{
		WorkingDirectory:        workingDir,
		IncludeTestDependencies: true,
		ExtraArgs:               extractResolutionArgs(userArgs),
	}

	// Create Maven FlexPack instance
	mavenFlex, err := flexpack.NewMavenFlexPack(config)
	if err != nil {
		return fmt.Errorf("failed to create Maven FlexPack: %w", err)
	}

	// Collect build info using FlexPack
	buildInfo, err := mavenFlex.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info with FlexPack: %w", err)
	}

	// For a deploy command, attach each module's deployed artifacts, then finalize them: record their
	// real deployment repository (OriginalDeploymentRepo) and tag them with build properties. This runs
	// BEFORE the build info is saved so OriginalDeploymentRepo is persisted for `rt bp`. It is
	// best-effort - artifact bookkeeping must not fail the build.
	if wasDeployCommand(userArgs) {
		addDeployedArtifactsToBuildInfo(buildInfo, mavenFlex.GetModuleLocations())
		// Resolve the deployment repositories from Maven's effective model (effective-pom/settings), so
		// they are correct under interpolation, inheritance and active profiles. moduleDeployURLs maps
		// each module to its own repo (reactors may deploy modules to different repos); overrideURL, when
		// set (-DaltDeploymentRepository / settings), applies to all modules.
		moduleDeployURLs, overrideURL, repoErr := mavenFlex.GetDeploymentRepositories()
		if repoErr != nil {
			log.Warn("Failed to resolve Maven deployment repository: " + repoErr.Error())
		}
		// Ensure the build's general details (start timestamp) exist before finalizing, so build.timestamp
		// resolves to the build's real timestamp. finalize runs before the build info is saved, so the
		// save's own GetOrCreateBuild has not recorded the timestamp yet.
		if _, buildErr := build.NewBuildInfoService().GetOrCreateBuildWithProject(buildName, buildNumber, buildConfiguration.GetProject()); buildErr != nil {
			log.Warn("Failed to initialize build details for timestamp: " + buildErr.Error())
		}
		if err := finalizeDeployedArtifacts(workingDir, buildInfo, moduleDeployURLs, overrideURL, buildName, buildNumber, buildConfiguration, serverDetails); err != nil {
			log.Warn("Failed to finalize deployed artifacts: " + err.Error())
		}
	}

	// Save FlexPack build info for jfrog-cli rt bp compatibility (following Poetry pattern)
	err = saveMavenFlexPackBuildInfo(buildInfo)
	if err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: " + err.Error())
	} else {
		log.Info("Build info saved locally. Use 'jf rt bp " + buildName + " " + buildNumber + "' to publish it to Artifactory.")
	}

	return nil
}

// saveMavenFlexPackBuildInfo saves Maven FlexPack build info for jfrog-cli rt bp compatibility
// This follows the exact same pattern as Poetry's saveFlexPackBuildInfo
func saveMavenFlexPackBuildInfo(buildInfo *entities.BuildInfo) error {
	// Create build-info service (same as Poetry)
	service := build.NewBuildInfoService()

	// Create or get build (same as Poetry)
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, "")
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}

	// Save the complete build info (this will be loaded by rt bp)
	return buildInstance.SaveBuildInfo(buildInfo)
}

// wasDeployCommand reports whether the Maven goals include a deploy goal. It inspects the parsed
// invocation passed in (the same userArgs forwarded for resolution-flag extraction) rather than the
// process-global os.Args, so the decision is testable and consistent with the rest of the flow.
func wasDeployCommand(userArgs []string) bool {
	for _, arg := range userArgs {
		// Match standalone "deploy" goal or any deploy plugin goal
		// Examples: deploy, deploy:deploy, deploy:deploy-file, maven-deploy-plugin:deploy
		if arg == "deploy" || strings.HasPrefix(arg, "deploy:") || strings.HasSuffix(arg, ":deploy") {
			return true
		}
	}
	return false
}

// finalizeDeployedArtifacts records each deployed artifact's real deployment repository on the build
// info (OriginalDeploymentRepo) and tags the deployed items with build.name/build.number/build.timestamp
// so Artifactory links them to the build (this resolves the artifact "path" in the build UI; without it
// the artifacts show as "externally resolved / No path found").
//
// deployRepoURL is Maven's EFFECTIVE deployment URL (resolved from effective-pom/effective-settings, so
// inheritance, interpolation and active profiles are already applied). Its repo key is resolved to the
// physical repository - a virtual repo becomes its default-deployment local repo, via GetRepository -
// and artifacts are then matched by sha256 scoped to that repo. That way identical content stored
// elsewhere is never mis-tagged, and no repository path layout is assumed. build.timestamp comes from
// the build's real timestamp via buildUtils.CreateBuildProperties.
//
// Must run BEFORE the build info is saved so OriginalDeploymentRepo is persisted for `rt bp`.
func finalizeDeployedArtifacts(workingDir string, buildInfo *entities.BuildInfo, moduleDeployURLs map[string]string, overrideURL, buildName, buildNumber string, buildArgs *buildUtils.BuildConfiguration, serverDetails *config.ServerDetails) error {
	if overrideURL == "" && len(moduleDeployURLs) == 0 {
		log.Debug("Could not determine any Maven deployment repository; skipping build-property tagging")
		return nil
	}

	// serverDetails comes from the resolved server-id (falls back to the default configured server).
	if serverDetails == nil {
		var err error
		if serverDetails, err = config.GetDefaultServerConf(); err != nil {
			return fmt.Errorf("failed to get server details: %w", err)
		}
	}
	if serverDetails == nil {
		log.Debug("No server details configured, skipping deployed-artifact finalization")
		return nil
	}
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	// Group each module's artifact checksums by the PHYSICAL repo it deployed to (resolving virtual ->
	// default-deployment). Modules may deploy to DIFFERENT repos, so this is per-module, not one repo
	// for the whole reactor. overrideURL (-DaltDeploymentRepository / settings) applies to every module.
	physicalByKey := make(map[string]string)   // repoKey -> physical repo (GetRepository cache)
	sha256sByRepo := make(map[string][]string) // physical repo -> checksums to tag
	for i := range buildInfo.Modules {
		module := &buildInfo.Modules[i]
		deployURL := overrideURL
		if deployURL == "" {
			deployURL = moduleDeployURLs[module.Id]
		}
		if deployURL == "" {
			log.Debug("No deployment repository for module " + module.Id + "; skipping its build-property tagging")
			continue
		}
		repoKey, keyErr := extractRepoKeyFromUrl(deployURL)
		if keyErr != nil {
			log.Warn("Skipping module " + module.Id + ": " + keyErr.Error())
			continue
		}
		physicalRepo, cached := physicalByKey[repoKey]
		if !cached {
			physicalRepo = resolvePhysicalDeployRepo(servicesManager, repoKey)
			physicalByKey[repoKey] = physicalRepo
		}
		if physicalRepo == "" {
			log.Warn("Could not resolve a physical deployment repository for '" + repoKey + "'; skipping module " + module.Id)
			continue
		}
		// Record the module's real deployment repo, and collect its checksums for tagging in that repo.
		for j := range module.Artifacts {
			if module.Artifacts[j].OriginalDeploymentRepo == "" {
				module.Artifacts[j].OriginalDeploymentRepo = physicalRepo
			}
			if module.Artifacts[j].Sha256 != "" {
				sha256sByRepo[physicalRepo] = append(sha256sByRepo[physicalRepo], module.Artifacts[j].Sha256)
			}
		}
	}

	if len(sha256sByRepo) == 0 {
		log.Warn("No deployed artifacts with checksums found; skipping build-property tagging")
		return nil
	}

	// One AQL + one (parallelized) SetProps per physical repo.
	buildProps := mavenBuildProperties(buildName, buildNumber, buildArgs.GetProject(), workingDir)
	total := 0
	for physicalRepo, sha256s := range sha256sByRepo {
		if len(sha256s) == 0 {
			continue
		}
		count, tagErr := tagArtifactsInRepo(servicesManager, physicalRepo, sha256s, buildProps)
		if tagErr != nil {
			log.Warn("Failed to set build properties in '" + physicalRepo + "': " + tagErr.Error())
			continue
		}
		total += count
	}
	if total == 0 {
		log.Warn("No deployed artifacts found to tag with build properties")
		return nil
	}
	log.Info(fmt.Sprintf("Set build properties on %d deployed Maven artifact(s)", total))
	return nil
}

// tagArtifactsInRepo tags every artifact in repo whose content matches one of sha256s with buildProps,
// using a single AQL search + a single SetProps call. Returns the number of artifacts tagged.
func tagArtifactsInRepo(servicesManager artifactory.ArtifactoryServicesManager, repo string, sha256s []string, buildProps string) (int, error) {
	reader, err := servicesManager.SearchFiles(services.SearchParams{
		CommonParams: &specutils.CommonParams{Aql: specutils.Aql{ItemsFind: checksumAql(repo, sha256s)}},
	})
	if err != nil {
		return 0, err
	}
	count, setErr := servicesManager.SetProps(services.PropsParams{Reader: reader, Props: buildProps})
	readerErr := reader.GetError()
	if closeErr := reader.Close(); closeErr != nil {
		log.Debug("Failed to close search reader: " + closeErr.Error())
	}
	if setErr != nil {
		return 0, setErr
	}
	if readerErr != nil {
		return 0, readerErr
	}
	return count, nil
}

// resolvePhysicalDeployRepo returns the physical repository that stores artifacts deployed to repoKey.
// If repoKey is a virtual repository, its configured defaultDeploymentRepo is returned (empty if none,
// which is unusable); otherwise repoKey is returned unchanged. Mirrors the pnpm/nix/docker FlexPack pattern.
func resolvePhysicalDeployRepo(servicesManager artifactory.ArtifactoryServicesManager, repoKey string) string {
	repoDetails := &services.VirtualRepositoryBaseParams{}
	if err := servicesManager.GetRepository(repoKey, repoDetails); err != nil {
		log.Debug(fmt.Sprintf("Could not read repository '%s', using as-is: %s", repoKey, err.Error()))
		return repoKey
	}
	if repoDetails.Rclass == services.VirtualRepositoryRepoType {
		if repoDetails.DefaultDeploymentRepo == "" {
			log.Warn("Virtual repository '" + repoKey + "' has no default deployment repository configured; " +
				"cannot tag deployed artifacts. Configure one, or deploy to a local repository.")
			return ""
		}
		log.Debug("Resolved virtual repository '" + repoKey + "' to default deployment repository '" + repoDetails.DefaultDeploymentRepo + "'")
		return repoDetails.DefaultDeploymentRepo
	}
	return repoKey
}

// extractRepoKeyFromUrl extracts the repository key from an Artifactory deployment URL, handling both
// the "/artifactory/<repo>" and "/artifactory/api/maven/<repo>" forms (the repo key is the last segment).
func extractRepoKeyFromUrl(repoUrl string) (string, error) {
	repoUrl = strings.TrimSpace(repoUrl)
	u, err := url.Parse(repoUrl)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	// /artifactory/api/maven/<REPO-KEY>
	if len(segments) >= 4 && segments[len(segments)-3] == "api" && segments[len(segments)-2] == "maven" {
		if repoKey := segments[len(segments)-1]; repoKey != "" {
			return repoKey, nil
		}
	}
	// /artifactory/<REPO-KEY>, or just <REPO-KEY> when Artifactory is at the host root.
	if repoKey := segments[len(segments)-1]; repoKey != "" {
		if strings.EqualFold(repoKey, "artifactory") {
			return "", fmt.Errorf("unable to extract repository key from URL (URL points to Artifactory root, not a specific repository): %s", repoUrl)
		}
		return repoKey, nil
	}
	return "", fmt.Errorf("unable to extract repository key from URL: %s", repoUrl)
}

// checksumAql builds an AQL find body matching any of the given sha256 checksums within repo. sha256 is
// the collision-resistant content identifier; scoping to the (physical) deploy repo keeps identical
// content stored elsewhere from being matched, without assuming any repository path layout.
func checksumAql(repo string, sha256s []string) string {
	orList := make([]map[string]string, len(sha256s))
	for i, sha256 := range sha256s {
		orList[i] = map[string]string{"sha256": sha256}
	}
	type aqlBody struct {
		Repo string              `json:"repo"`
		Or   []map[string]string `json:"$or"`
	}
	b, _ := json.Marshal(aqlBody{Repo: repo, Or: orList})
	return string(b)
}

// mavenBuildProperties builds the build.name;build.number;build.timestamp property string (timestamp is
// the build's real timestamp, matching the published build-info and the docker build/buildx convention),
// plus the optional build.project and any user-configured properties.
func mavenBuildProperties(buildName, buildNumber, projectKey, workingDir string) string {
	buildProps, err := buildUtils.CreateBuildProperties(buildName, buildNumber, projectKey)
	if err != nil {
		log.Debug("Build timestamp unavailable, tagging with name/number only: " + err.Error())
	}
	if projectKey != "" {
		buildProps += fmt.Sprintf(";build.project=%s", projectKey)
	}
	return civcs.MergeWithUserProps(buildProps, workingDir)
}

// mavenCoordinateRegex is an allowlist of the only characters that may legitimately appear in a
// Maven groupId/artifactId/version/packaging value (and in the composed "<artifactId>-<version>.<packaging>"
// filename, which additionally contains the '-' and '.' separators already covered below).
var mavenCoordinateRegex = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateMavenCoordinate validates a value read from a user-controlled pom.xml using a strict
// allowlist, rejecting anything that is not a legitimate Maven coordinate.
//
// Background: Maven coordinates (groupId, artifactId, version) and the packaging type are read
// straight from a user-controlled pom.xml and later composed into a filename that is joined with
// the project's "target" directory (see addDeployedArtifactsToBuildInfo). Without validation, a
// crafted pom.xml could inject values like "../../etc" and cause the resulting filepath.Join to
// escape the target directory (path traversal).
//
// An allowlist is used in preference to a denylist: legitimate Maven coordinates only ever use the
// characters [A-Za-z0-9._-], so anything outside that set (path separators, null bytes, newlines,
// shell metacharacters, encoded sequences, ...) is rejected without having to enumerate every
// bypass vector. The "." is allowed because it is the package/version separator, so we additionally
// reject the ".." traversal sequence explicitly (it is otherwise composed entirely of allowed
// characters). The relative-path form ("../pom.xml") only appears in the <parent><relativePath>
// element, which this code does not parse and never feeds into a path, so this cannot reject a
// valid project.
//
// The same helper is reused to validate the composed artifact filename (defense in depth) so the
// "trailing dot in version" edge case (e.g. version "1.0." -> "app-1.0..jar") is caught in one place.
func validateMavenCoordinate(value string) error {
	if value == "" {
		return fmt.Errorf("value is empty")
	}
	if strings.Contains(value, "..") {
		return fmt.Errorf("value %q contains path traversal sequence", value)
	}
	if !mavenCoordinateRegex.MatchString(value) {
		return fmt.Errorf("value %q contains characters not permitted in Maven coordinates", value)
	}
	return nil
}

// isSeparateValueFlag reports whether arg is a resolution flag that consumes the next token as its value.
func isSeparateValueFlag(arg string) bool {
	return arg == "-s" || arg == "--settings" ||
		arg == "-f" || arg == "--file" ||
		arg == "-gs" || arg == "--global-settings"
}

// extractResolutionArgs picks the resolution-affecting flags out of the user's Maven invocation so
// they can be replayed on the internal `mvn dependency:tree` call. Forwarded flags:
//   - -P / --activate-profiles     active profiles
//   - -D / --define                system property overrides
//   - -s / --settings              user settings file (value in next token or = form)
//   - -f / --file                  alternate POM (changes what gets resolved)
//   - -gs / --global-settings      global settings file
//   - -o / --offline               offline mode
//
// Everything else (goals, deploy-only flags, etc.) is ignored.
func extractResolutionArgs(userArgs []string) []string {
	var extracted []string
	for i := 0; i < len(userArgs); i++ {
		arg := userArgs[i]
		switch {
		case len(arg) > 2 && strings.HasPrefix(arg, "-P"), strings.HasPrefix(arg, "--activate-profiles="),
			len(arg) > 2 && strings.HasPrefix(arg, "-D"), strings.HasPrefix(arg, "--define="),
			strings.HasPrefix(arg, "--settings="), strings.HasPrefix(arg, "-f="),
			strings.HasPrefix(arg, "--file="), strings.HasPrefix(arg, "-gs="),
			strings.HasPrefix(arg, "--global-settings="),
			arg == "-o", arg == "--offline":
			extracted = append(extracted, arg)
		case isSeparateValueFlag(arg),
			arg == "-P", arg == "-D", arg == "--activate-profiles", arg == "--define":
			extracted = append(extracted, arg)
			// These flags take their value as the next token.
			if i+1 < len(userArgs) {
				i++
				extracted = append(extracted, userArgs[i])
			}
		}
	}
	return extracted
}

// addDeployedArtifactsToBuildInfo attaches each module's deployed artifacts to the matching build-info
// module. locations is FlexPack's authoritative id -> location map (built from what Maven actually
// ran, profile-activated modules included), so no pom re-discovery is needed and submodule artifacts
// are no longer dropped (previously only Modules[0] received artifacts).
func addDeployedArtifactsToBuildInfo(buildInfo *entities.BuildInfo, locations map[string]flexpack.ModuleLocation) {
	if len(buildInfo.Modules) == 0 {
		log.Warn("No modules found in build info, cannot add artifacts")
		return
	}

	for i := range buildInfo.Modules {
		module := &buildInfo.Modules[i]
		location, ok := locations[module.Id]
		if !ok {
			log.Debug("No build location found for module " + module.Id + ", skipping artifact collection")
			continue
		}
		if artifacts := collectModuleArtifacts(module.Id, location); len(artifacts) > 0 {
			module.Artifacts = artifacts
		}
	}
}

// collectModuleArtifacts collects the artifacts a single module produces: its main artifact (matching
// the packaging) from target/, and its pom.xml. Coordinates come from the module id and the build
// directory/packaging from FlexPack's recorded location, so no pom.xml is re-parsed. Modules without a
// target directory (e.g. a pom aggregator) yield only the pom artifact.
func collectModuleArtifacts(moduleId string, location flexpack.ModuleLocation) []entities.Artifact {
	groupId, artifactId, version, ok := splitModuleId(moduleId)
	if !ok {
		log.Warn("Skipping artifacts for module with invalid id: " + moduleId)
		return nil
	}

	var artifacts []entities.Artifact

	// Main artifact matching the packaging type (only present for buildable modules, not pom aggregators).
	// This follows traditional Maven behavior where intermediate build artifacts (e.g., .jar in WAR projects) are excluded.
	targetDir := filepath.Join(location.Dir, "target")
	if _, statErr := os.Stat(targetDir); statErr == nil {
		packagingType := sanitizePackaging(location.Packaging)
		mainArtifactName := fmt.Sprintf("%s-%s.%s", artifactId, version, packagingFileExtension(packagingType))
		// Defense in depth: re-validate the composed filename before joining it with targetDir.
		// The individual inputs are already sanitized at extraction, but composing them (version "." packaging)
		// could still produce ".." at a boundary (e.g. a trailing-dot version "1.0." -> "app-1.0..jar").
		// Reusing validateMavenCoordinate keeps the traversal rules in a single place. An invalid
		// composed name means we skip the main artifact rather than fail the whole build.
		if validateErr := validateMavenCoordinate(mainArtifactName); validateErr != nil {
			log.Warn("Skipping main artifact for module " + moduleId + ": " + validateErr.Error())
		} else {
			// filepath.Base strips any directory component, guaranteeing the artifact is read from directly
			// within targetDir regardless of the (already validated) input. This collapses the value to a
			// single path element and is the canonical sanitizer for stored path-traversal data flows.
			mainArtifactName = filepath.Base(mainArtifactName)
			mainArtifactPath := filepath.Join(targetDir, mainArtifactName)

			if _, statErr := os.Stat(mainArtifactPath); statErr == nil {
				artifacts = append(artifacts, createArtifactFromFile(mainArtifactPath, groupId, artifactId, version, packagingType))
			}
		}
	}

	// POM artifact (from the module root, not target).
	pomArtifactPath := filepath.Join(location.Dir, "pom.xml")
	if _, statErr := os.Stat(pomArtifactPath); statErr == nil {
		artifacts = append(artifacts, createArtifactFromFile(pomArtifactPath, groupId, artifactId, version, "pom"))
	}

	return artifacts
}

// splitModuleId splits a "groupId:artifactId:version" module id into its parts and validates each as a
// Maven coordinate (the parts are later composed into filenames, so path-traversal input is rejected).
func splitModuleId(moduleId string) (groupId, artifactId, version string, ok bool) {
	parts := strings.Split(moduleId, ":")
	if len(parts) != 3 {
		return "", "", "", false
	}
	for _, p := range parts {
		if validateMavenCoordinate(p) != nil {
			return "", "", "", false
		}
	}
	return parts[0], parts[1], parts[2], true
}

// packagingFileExtension maps Maven packaging types that produce .jar files (maven-plugin, bundle, ejb,
// maven-archetype) to the "jar" extension used in the deployed filename, while other types use their
// own name as the extension. The artifact Type field still records the original packaging type.
func packagingFileExtension(packaging string) string {
	switch packaging {
	case "maven-plugin", "bundle", "ejb", "maven-archetype":
		return "jar"
	default:
		return packaging
	}
}

// sanitizePackaging validates a packaging value (recorded from the dependency-tree root node) that is
// composed into an artifact filename, falling back to Maven's default "jar" when empty or unsafe.
func sanitizePackaging(packaging string) string {
	if packaging == "" {
		return "jar"
	}
	if validateMavenCoordinate(packaging) != nil {
		log.Warn("Invalid packaging '" + packaging + "', falling back to jar")
		return "jar"
	}
	return packaging
}

// createArtifactFromFile creates an entities.Artifact from a file path
func createArtifactFromFile(filePath, groupId, artifactId, version, artifactType string) entities.Artifact {
	// Calculate file checksums using crypto.GetFileDetails
	fileDetails, err := crypto.GetFileDetails(filePath, true)
	if err != nil {
		log.Debug("Failed to calculate checksums for " + filePath + ": " + err.Error())
		// Continue with empty checksums rather than failing
		fileDetails = &crypto.FileDetails{}
	}

	// Create artifact name and path
	fileName := filepath.Base(filePath)
	if artifactType == "pom" {
		fileName = fmt.Sprintf("%s-%s.pom", artifactId, version)
	}

	artifactPath := fmt.Sprintf("%s/%s/%s/%s", strings.ReplaceAll(groupId, ".", "/"), artifactId, version, fileName)

	artifact := entities.Artifact{
		Name: fileName,
		Path: artifactPath,
		Type: artifactType,
		Checksum: entities.Checksum{
			Md5:    fileDetails.Checksum.Md5,
			Sha1:   fileDetails.Checksum.Sha1,
			Sha256: fileDetails.Checksum.Sha256,
		},
	}

	return artifact
}
