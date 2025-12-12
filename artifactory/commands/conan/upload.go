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

	// Extract remote name and set target repo
	remoteName := extractRemoteNameFromOutput(uploadOutput)
	if remoteName == "" {
		remoteName = "conan-local"
	}

	// Collect artifacts from Artifactory
	if up.serverDetails != nil {
		artifacts, err := up.collectArtifacts(packageRef, remoteName)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to collect artifacts: %v", err))
		} else {
			up.addArtifactsToModule(buildInfo, artifacts)
		}

		// Set build properties on artifacts
		if len(artifacts) > 0 {
			if err := up.setBuildProperties(artifacts, remoteName); err != nil {
				log.Warn(fmt.Sprintf("Failed to set build properties: %v", err))
			}
		}
	}

	return up.saveBuildInfo(buildInfo)
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

	// Fallback: look for "Uploading recipe" pattern
	return up.parseUploadPattern(lines)
}

// parseUploadPattern looks for package reference in upload lines.
func (up *UploadProcessor) parseUploadPattern(lines []string) string {
	for _, line := range lines {
		if strings.Contains(line, "Uploading recipe") {
			// Extract package reference from: "Uploading recipe 'name/version#rev'"
			start := strings.Index(line, "'")
			end := strings.LastIndex(line, "'")
			if start != -1 && end > start {
				ref := line[start+1 : end]
				// Remove revision if present
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

	log.Info(fmt.Sprintf("Collected build info with %d modules", len(buildInfo.Modules)))
	if len(buildInfo.Modules) > 0 {
		log.Info(fmt.Sprintf("Module '%s' has %d dependencies",
			buildInfo.Modules[0].Id, len(buildInfo.Modules[0].Dependencies)))
	}

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

// collectArtifacts collects artifacts from Artifactory.
func (up *UploadProcessor) collectArtifacts(packageRef, remoteName string) ([]entities.Artifact, error) {
	collector := NewArtifactCollector(up.serverDetails, remoteName)
	artifacts, err := collector.CollectArtifacts(packageRef)
	if err != nil {
		return nil, err
	}

	log.Info(fmt.Sprintf("Found %d Conan artifacts in Artifactory", len(artifacts)))
	return artifacts, nil
}

// addArtifactsToModule adds artifacts to the first module in build info.
func (up *UploadProcessor) addArtifactsToModule(buildInfo *entities.BuildInfo, artifacts []entities.Artifact) {
	if len(buildInfo.Modules) == 0 {
		return
	}

	buildInfo.Modules[0].Artifacts = artifacts
	log.Info(fmt.Sprintf("Module '%s' now has %d dependencies and %d artifacts",
		buildInfo.Modules[0].Id,
		len(buildInfo.Modules[0].Dependencies),
		len(buildInfo.Modules[0].Artifacts)))
}

// setBuildProperties sets build properties on artifacts in Artifactory.
func (up *UploadProcessor) setBuildProperties(artifacts []entities.Artifact, remoteName string) error {
	buildName, err := up.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}

	buildNumber, err := up.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	projectKey := up.buildConfiguration.GetProject()

	setter := NewBuildPropertySetter(up.serverDetails, remoteName, buildName, buildNumber, projectKey)
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

	log.Info("Successfully saved Conan build info")
	return nil
}

// extractRemoteNameFromOutput extracts the remote name from conan upload output.
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

// FlexPackCollector wraps the FlexPack Conan collector.
type FlexPackCollector struct {
	config conanflex.ConanConfig
}

// NewFlexPackCollector creates a new FlexPack collector.
func NewFlexPackCollector(workingDir string) (*FlexPackCollector, error) {
	return &FlexPackCollector{
		config: conanflex.ConanConfig{
			WorkingDirectory: workingDir,
		},
	}, nil
}

// CollectBuildInfo collects build info using FlexPack.
func (fc *FlexPackCollector) CollectBuildInfo(buildName, buildNumber string) (*entities.BuildInfo, error) {
	collector, err := conanflex.NewConanFlexPack(fc.config)
	if err != nil {
		return nil, fmt.Errorf("create conan flexpack: %w", err)
	}

	return collector.CollectBuildInfo(buildName, buildNumber)
}

