package flexpack

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// CollectMavenBuildInfoWithFlexPack collects Maven build info using FlexPack
// This follows the same pattern as Poetry FlexPack in poetry.go
func CollectMavenBuildInfoWithFlexPack(workingDir, buildName, buildNumber string, buildConfiguration *buildUtils.BuildConfiguration) error {
	// Create Maven FlexPack configuration (following Poetry pattern)
	config := flexpack.MavenConfig{
		WorkingDirectory:        workingDir,
		IncludeTestDependencies: true,
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

	// Add deployed artifacts to build info if this was a deploy command
	if wasDeployCommand() {
		err = addDeployedArtifactsToBuildInfo(buildInfo, workingDir)
		if err != nil {
			log.Warn("Failed to add deployed artifacts to build info: " + err.Error())
		}
	}

	// Save FlexPack build info for jfrog-cli rt bp compatibility (following Poetry pattern)
	err = saveMavenFlexPackBuildInfo(buildInfo)
	if err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: " + err.Error())
	} else {
		log.Info("Build info saved locally. Use 'jf rt bp " + buildName + " " + buildNumber + "' to publish it to Artifactory.")
	}

	// Set build properties on deployed artifacts if this was a deploy command
	if wasDeployCommand() {
		err = setMavenBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber, buildConfiguration)
		if err != nil {
			log.Warn("Failed to set build properties on deployed artifacts: " + err.Error())
			// Don't fail the entire operation for property setting issues
		}
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

// wasDeployCommand checks if the current command was a Maven deploy command
func wasDeployCommand() bool {
	args := os.Args
	for _, arg := range args {
		if arg == "deploy" {
			return true
		}
	}
	return false
}

// setMavenBuildPropertiesOnArtifacts sets build properties on deployed Maven artifacts
// Following the pattern from twine.go
func setMavenBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber string, buildArgs *buildUtils.BuildConfiguration) error {
	log.Debug("Setting build properties on deployed Maven artifacts...")

	// Get server details from configuration
	serverDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return fmt.Errorf("failed to get server details: %w", err)
	}

	if serverDetails == nil {
		log.Debug("No server details configured, skipping build properties setting")
		return nil
	}

	// Create services manager
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	// Get Maven artifact info from pom.xml
	groupId, artifactId, version, err := getMavenArtifactCoordinates(workingDir)
	if err != nil {
		return fmt.Errorf("failed to get Maven artifact coordinates: %w", err)
	}

	// Create search pattern for the specific deployed artifacts
	// Search for artifacts in maven-flexpack-local repository with the specific coordinates
	artifactPath := fmt.Sprintf("maven-flexpack-local/%s/%s/%s/%s-*",
		strings.ReplaceAll(groupId, ".", "/"), artifactId, version, artifactId)

	log.Debug("Searching for deployed artifacts with pattern: " + artifactPath)

	// Search for deployed artifacts using the specific pattern
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Pattern: artifactPath,
		},
	}

	searchReader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		return fmt.Errorf("failed to search for deployed artifacts: %w", err)
	}
	defer searchReader.Close()

	// Create build properties in the same format as NPM/traditional implementations
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10) // Unix milliseconds like NPM
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if projectKey := buildArgs.GetProject(); projectKey != "" {
		buildProps += fmt.Sprintf(";build.project=%s", projectKey)
	}

	// Set build properties on found artifacts
	propsParams := services.PropsParams{
		Reader: searchReader,
		Props:  buildProps,
	}

	_, err = servicesManager.SetProps(propsParams)
	if err != nil {
		return fmt.Errorf("failed to set build properties on artifacts: %w", err)
	}

	log.Info("Successfully set build properties on deployed Maven artifacts")
	return nil
}

// getMavenArtifactCoordinates extracts Maven coordinates from pom.xml
func getMavenArtifactCoordinates(workingDir string) (groupId, artifactId, version string, err error) {
	// Read pom.xml to get project information
	pomPath := filepath.Join(workingDir, "pom.xml")
	pomData, err := os.ReadFile(pomPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read pom.xml: %w", err)
	}

	pomContent := string(pomData)

	// Extract groupId
	if start := strings.Index(pomContent, "<groupId>"); start != -1 {
		start += len("<groupId>")
		if end := strings.Index(pomContent[start:], "</groupId>"); end != -1 {
			groupId = strings.TrimSpace(pomContent[start : start+end])
		}
	}

	// Extract artifactId
	if start := strings.Index(pomContent, "<artifactId>"); start != -1 {
		start += len("<artifactId>")
		if end := strings.Index(pomContent[start:], "</artifactId>"); end != -1 {
			artifactId = strings.TrimSpace(pomContent[start : start+end])
		}
	}

	// Extract version
	if start := strings.Index(pomContent, "<version>"); start != -1 {
		start += len("<version>")
		if end := strings.Index(pomContent[start:], "</version>"); end != -1 {
			version = strings.TrimSpace(pomContent[start : start+end])
		}
	}

	if groupId == "" || artifactId == "" || version == "" {
		return "", "", "", fmt.Errorf("failed to extract complete Maven coordinates from pom.xml (groupId=%s, artifactId=%s, version=%s)", groupId, artifactId, version)
	}

	return groupId, artifactId, version, nil
}

// addDeployedArtifactsToBuildInfo adds deployed artifacts to the build info
func addDeployedArtifactsToBuildInfo(buildInfo *entities.BuildInfo, workingDir string) error {
	log.Debug("Adding deployed artifacts to build info...")

	// Find the target directory with built artifacts
	targetDir := filepath.Join(workingDir, "target")
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		log.Debug("No target directory found, skipping artifact collection")
		return nil
	}

	// Get Maven artifact coordinates
	groupId, artifactId, version, err := getMavenArtifactCoordinates(workingDir)
	if err != nil {
		return fmt.Errorf("failed to get Maven artifact coordinates: %w", err)
	}

	// Create artifacts for the deployed files
	var artifacts []entities.Artifact

	// Add main artifact (jar/war/etc)
	mainArtifactName := fmt.Sprintf("%s-%s.jar", artifactId, version)
	mainArtifactPath := filepath.Join(targetDir, mainArtifactName)

	if _, err := os.Stat(mainArtifactPath); err == nil {
		artifact := createArtifactFromFile(mainArtifactPath, groupId, artifactId, version, "jar")
		artifacts = append(artifacts, artifact)
	}

	// Add POM artifact
	pomArtifactName := fmt.Sprintf("%s-%s.pom", artifactId, version)
	// POM is usually in the project root, not target directory for deployment
	pomArtifactPath := filepath.Join(workingDir, "pom.xml")

	if _, err := os.Stat(pomArtifactPath); err == nil {
		artifact := createArtifactFromFile(pomArtifactPath, groupId, artifactId, version, "pom")
		// Set the correct name and path for the deployed POM
		artifact.Name = pomArtifactName
		artifact.Path = fmt.Sprintf("%s/%s/%s/%s", strings.ReplaceAll(groupId, ".", "/"), artifactId, version, pomArtifactName)
		artifacts = append(artifacts, artifact)
	}

	// Add artifacts to the first module (Maven projects typically have one module)
	if len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Artifacts = artifacts
		log.Debug(fmt.Sprintf("Added %d artifacts to build info", len(artifacts)))
	} else {
		log.Warn("No modules found in build info, cannot add artifacts")
	}

	return nil
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
