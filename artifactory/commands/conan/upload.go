// Package conan provides Conan package manager integration for JFrog Artifactory.
package conan

import (
	"fmt"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	conanflex "github.com/jfrog/build-info-go/flexpack/conan"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// UploadProcessor processes Conan upload output and collects build info.
// It parses the upload output to:
// 1. Extract which artifacts were uploaded (to avoid collecting all revisions)
// 2. Collect dependencies from the local project
// 3. Set build properties on uploaded artifacts
type UploadProcessor struct {
	workingDir         string
	buildConfiguration *buildUtils.BuildConfiguration
	serverDetails      *config.ServerDetails
}

// NewUploadProcessor creates a new upload processor.
func NewUploadProcessor(workingDir string, buildConfig *buildUtils.BuildConfiguration, serverDetails *config.ServerDetails) *UploadProcessor {
	return &UploadProcessor{
		workingDir:         workingDir,
		buildConfiguration: buildConfig,
		serverDetails:      serverDetails,
	}
}

// Process processes the upload output and collects build info.
func (up *UploadProcessor) Process(uploadOutput string) error {
	// Parse uploaded artifacts from output - only collect what was actually uploaded
	uploadedPaths := up.parseUploadedArtifactPaths(uploadOutput)
	log.Debug(fmt.Sprintf("Found %d uploaded artifact paths", len(uploadedPaths)))

	// Parse package reference from upload output
	packageRef := up.parsePackageReference(uploadOutput)
	if packageRef == "" {
		log.Debug("No package reference found in upload output")
		return nil
	}
	log.Debug(fmt.Sprintf("Processing upload for package: %s", packageRef))

	// Collect dependencies using FlexPack
	buildInfo, err := up.collectDependencies()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to collect dependencies: %v", err))
		buildInfo = up.createEmptyBuildInfo(packageRef)
	}

	// Get target repository from upload output
	targetRepo, err := up.getTargetRepository(uploadOutput)
	if err != nil {
		log.Warn(fmt.Sprintf("Could not determine target repository: %v", err))
		log.Warn("Build info will be saved but artifacts may not be linked correctly")
		return up.saveBuildInfo(buildInfo)
	}
	log.Debug(fmt.Sprintf("Using Artifactory repository: %s", targetRepo))

	// Collect artifacts from Artifactory - only for uploaded paths
	if up.serverDetails != nil && len(uploadedPaths) > 0 {
		artifacts, collectErr := up.collectUploadedArtifacts(uploadedPaths, targetRepo)
		if collectErr != nil {
			log.Warn(fmt.Sprintf("Failed to collect artifacts: %v", collectErr))
		} else {
			up.addArtifactsToModule(buildInfo, artifacts)

			// Set build properties on artifacts
			if len(artifacts) > 0 {
				if err := up.setBuildProperties(artifacts, targetRepo); err != nil {
					log.Warn(fmt.Sprintf("Failed to set build properties: %v", err))
				}
			}
		}
	}

	return up.saveBuildInfo(buildInfo)
}

// getTargetRepository extracts the Conan remote from upload output and resolves it to Artifactory repo.
// Returns error if remote cannot be determined or is not an Artifactory repository.
func (up *UploadProcessor) getTargetRepository(uploadOutput string) (string, error) {
	remoteName := extractRemoteNameFromOutput(uploadOutput)
	if remoteName == "" {
		return "", fmt.Errorf("could not extract remote name from upload output")
	}

	// Verify this is an Artifactory Conan remote
	remoteURL, err := getRemoteURL(remoteName)
	if err != nil {
		return "", fmt.Errorf("could not get URL for remote '%s': %w", remoteName, err)
	}

	if !isArtifactoryConanRemote(remoteURL) {
		return "", fmt.Errorf("remote '%s' is not an Artifactory Conan repository (URL: %s)", remoteName, remoteURL)
	}

	// Get the Artifactory repository name from the URL
	repoName := ExtractRepoName(remoteURL)
	if repoName == "" {
		return "", fmt.Errorf("could not extract repository name from URL: %s", remoteURL)
	}

	return repoName, nil
}

// parseUploadedArtifactPaths extracts the specific paths that were uploaded from conan upload output.
// Conan 2.x upload summary format:
//
//	Upload summary:
//	  conan-local-testing-reshmi      <- Remote name
//	    multideps/1.0.0               <- Package name/version
//	      revisions
//	        797d134a8590a1bfa06d846768443f48 (Uploaded)  <- Recipe revision
//	          packages
//	            594ed0eb2e9dfcc60607438924c35871514e6c2a  <- Package ID
//	              revisions
//	                ca858ea14c32f931e49241df0b52bec9 (Uploaded)  <- Package revision
//
// This method parses this structure and builds Artifactory paths like:
//   - _/multideps/1.0.0/_/797d134.../export (for recipe)
//   - _/multideps/1.0.0/_/797d134.../package/594ed0.../ca858ea... (for package)
func (up *UploadProcessor) parseUploadedArtifactPaths(output string) []string {
	var paths []string
	lines := strings.Split(output, "\n")

	// State tracking for hierarchical parsing
	var currentPkg string       // Current package name/version (e.g., "multideps/1.0.0")
	var currentRecipeRev string // Current recipe revision hash (MD5, 32 chars)
	var currentPkgId string     // Current package ID (SHA1, 40 chars)
	inUploadSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Start parsing after "Upload summary" or "Uploading to remote"
		if strings.Contains(line, "Upload summary") || strings.Contains(line, "Uploading to remote") {
			inUploadSection = true
			continue
		}

		if !inUploadSection {
			continue
		}

		// Skip empty lines and section markers
		if trimmed == "" || trimmed == "revisions" || trimmed == "packages" {
			continue
		}

		// Match package name/version line: "multideps/1.0.0"
		// Must contain "/" but not be a path or special marker
		if up.isPackageNameLine(trimmed) {
			currentPkg = trimmed
			currentRecipeRev = ""
			currentPkgId = ""
			continue
		}

		// Match recipe/package revision with (Uploaded) or (Skipped, already in server)
		// Both cases mean the artifact is in Artifactory and should be part of build info
		if strings.Contains(trimmed, "(Uploaded)") || strings.Contains(trimmed, "(Skipped") {
			// Extract revision hash by removing status suffix
			rev := trimmed
			if idx := strings.Index(rev, " ("); idx != -1 {
				rev = rev[:idx]
			}
			rev = strings.TrimSpace(rev)

			if len(rev) == 32 {
				if currentPkgId == "" {
					// This is a recipe revision
					currentRecipeRev = rev
					if currentPkg != "" {
						path := fmt.Sprintf("_/%s/_/%s/export", currentPkg, rev)
						paths = append(paths, path)
					}
				} else if currentRecipeRev != "" {
					// This is a package revision
					if currentPkg != "" {
						path := fmt.Sprintf("_/%s/_/%s/package/%s/%s", currentPkg, currentRecipeRev, currentPkgId, rev)
						paths = append(paths, path)
					}
					currentPkgId = "" // Reset for next package
				}
			}
			continue
		}

		// Match package ID line (SHA1, 40 chars, no spaces, no parentheses)
		if len(trimmed) == 40 && !strings.Contains(trimmed, " ") && !strings.Contains(trimmed, "(") {
			currentPkgId = trimmed
		}
	}

	return paths
}

// isPackageNameLine checks if a line represents a package name/version.
func (up *UploadProcessor) isPackageNameLine(line string) bool {
	return strings.Contains(line, "/") &&
		!strings.Contains(line, "#") &&
		!strings.Contains(line, ":") &&
		!strings.HasPrefix(line, "_") &&
		!strings.Contains(line, "Uploading") &&
		!strings.Contains(line, "Skipped") &&
		!strings.Contains(line, "(")
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

// parsePackageReference extracts package reference from upload output.
func (up *UploadProcessor) parsePackageReference(output string) string {
	lines := strings.Split(output, "\n")
	inSummary := false
	foundRemote := false

	for _, line := range lines {
		if strings.Contains(line, "Upload summary") {
			inSummary = true
			continue
		}

		if !inSummary {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}

		// Skip remote name line
		if !foundRemote {
			foundRemote = true
			continue
		}

		// Look for package reference pattern: name/version
		if strings.Contains(trimmed, "/") && !strings.Contains(trimmed, ":") {
			return trimmed
		}
	}

	// Fallback: look for "Uploading recipe" pattern (older Conan output format)
	return up.parseUploadingRecipePattern(lines)
}

// parseUploadingRecipePattern extracts package reference from "Uploading recipe" lines.
// Example: "Uploading recipe 'simplelib/1.0.0#86deb56...'"
func (up *UploadProcessor) parseUploadingRecipePattern(lines []string) string {
	for _, line := range lines {
		if strings.Contains(line, "Uploading recipe") {
			start := strings.Index(line, "'")
			end := strings.LastIndex(line, "'")
			if start != -1 && end > start {
				ref := line[start+1 : end]
				// Remove revision if present (after #)
				if hashIdx := strings.Index(ref, "#"); hashIdx != -1 {
					ref = ref[:hashIdx]
				}
				return ref
			}
		}
	}
	return ""
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

	conanConfig := conanflex.ConanConfig{
		WorkingDirectory: up.workingDir,
	}

	collector, err := conanflex.NewConanFlexPack(conanConfig)
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
	service := build.NewBuildInfoService()

	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, "")
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}

	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}

	log.Info("Conan build info saved locally")
	return nil
}

// extractRemoteNameFromOutput extracts the remote name from conan upload output.
// Looks in the "Upload summary" section for the remote name.
func extractRemoteNameFromOutput(output string) string {
	lines := strings.Split(output, "\n")
	inSummary := false

	for _, line := range lines {
		if strings.Contains(line, "Upload summary") {
			inSummary = true
			continue
		}

		if !inSummary {
			continue
		}

		trimmed := strings.TrimSpace(line)
		// First non-empty, non-dashed line after summary is the remote name
		if trimmed != "" && !strings.HasPrefix(trimmed, "-") && !strings.Contains(trimmed, "/") {
			return trimmed
		}
	}
	return ""
}
