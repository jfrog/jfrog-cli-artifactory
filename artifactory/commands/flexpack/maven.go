package flexpack

import (
	"encoding/xml"
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

// PomProject represents the Maven POM XML structure for parsing
type PomProject struct {
	XMLName                xml.Name               `xml:"project"`
	GroupId                string                 `xml:"groupId"`
	ArtifactId             string                 `xml:"artifactId"`
	Version                string                 `xml:"version"`
	Packaging              string                 `xml:"packaging"`
	Parent                 PomParent              `xml:"parent"`
	Properties             map[string]string      `xml:"properties"`
	DistributionManagement DistributionManagement `xml:"distributionManagement"`
}

type PomParent struct {
	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type DistributionManagement struct {
	Repository         Repository `xml:"repository"`
	SnapshotRepository Repository `xml:"snapshotRepository"`
}

type Repository struct {
	Id  string `xml:"id"`
	URL string `xml:"url"`
}

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
		// Match standalone "deploy" goal or plugin notation "maven-deploy-plugin:deploy"
		if arg == "deploy" || strings.HasSuffix(arg, ":deploy") {
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

	// Get the repository Maven deployed to from settings.xml or pom.xml
	targetRepo, err := getMavenDeployRepository(workingDir)
	if err != nil {
		log.Warn("Could not determine Maven deploy repository, skipping build properties: " + err.Error())
		return nil
	}

	// Create search pattern for the specific deployed artifacts in the target repository
	artifactPath := fmt.Sprintf("%s/%s/%s/%s/%s-*",
		targetRepo,
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

// getMavenDeployRepository determines where Maven deployed artifacts
// by parsing pom.xml distributionManagement
func getMavenDeployRepository(workingDir string) (string, error) {
	pomPath := filepath.Join(workingDir, "pom.xml")
	pomData, err := os.ReadFile(pomPath)
	if err != nil {
		return "", fmt.Errorf("failed to read pom.xml: %w", err)
	}

	var pom PomProject
	if err := xml.Unmarshal(pomData, &pom); err != nil {
		return "", fmt.Errorf("failed to parse pom.xml: %w", err)
	}

	// Get repository URL from distributionManagement
	var repoUrl string
	if pom.DistributionManagement.Repository.URL != "" {
		repoUrl = pom.DistributionManagement.Repository.URL
	} else if pom.DistributionManagement.SnapshotRepository.URL != "" {
		repoUrl = pom.DistributionManagement.SnapshotRepository.URL
	}

	if repoUrl == "" {
		return "", fmt.Errorf("no distributionManagement repository found in pom.xml")
	}

	// Extract repository key from URL
	// URL format: http://host:port/artifactory/REPO-KEY or http://host:port/artifactory/api/maven/REPO-KEY
	repoUrl = strings.TrimSpace(repoUrl)

	// Handle different URL patterns
	if strings.Contains(repoUrl, "/api/maven/") {
		// Format: http://host/artifactory/api/maven/REPO-KEY
		parts := strings.Split(repoUrl, "/api/maven/")
		if len(parts) == 2 {
			repoKey := strings.Trim(parts[1], "/")
			if repoKey != "" {
				log.Debug("Found deploy repository from pom.xml: " + repoKey)
				return repoKey, nil
			}
		}
	}

	// Standard format: http://host/artifactory/REPO-KEY
	if idx := strings.LastIndex(repoUrl, "/"); idx != -1 {
		repoKey := repoUrl[idx+1:]
		if repoKey != "" {
			log.Debug("Found deploy repository from pom.xml: " + repoKey)
			return repoKey, nil
		}
	}

	return "", fmt.Errorf("could not extract repository key from URL: %s", repoUrl)
}

// getMavenArtifactCoordinates extracts Maven coordinates from pom.xml
func getMavenArtifactCoordinates(workingDir string) (groupId, artifactId, version string, err error) {
	pomPath := filepath.Join(workingDir, "pom.xml")
	pomData, err := os.ReadFile(pomPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read pom.xml: %w", err)
	}

	var pom PomProject
	if err := xml.Unmarshal(pomData, &pom); err != nil {
		return "", "", "", fmt.Errorf("failed to parse pom.xml: %w", err)
	}

	// Use project values, fallback to parent if missing
	groupId = pom.GroupId
	if groupId == "" {
		groupId = pom.Parent.GroupId
	}

	artifactId = pom.ArtifactId

	version = pom.Version
	if version == "" {
		version = pom.Parent.Version
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

	// Get packaging type from pom.xml
	packagingType := getPackagingType(workingDir)

	// Create artifacts for the deployed files
	var artifacts []entities.Artifact

	// Scan target directory for all artifacts matching the pattern: artifactId-version*
	artifactPrefix := fmt.Sprintf("%s-%s", artifactId, version)
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return fmt.Errorf("failed to read target directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()

		// Only process files that match the artifact pattern
		if !strings.HasPrefix(fileName, artifactPrefix) {
			continue
		}

		// Determine artifact type from file extension
		ext := filepath.Ext(fileName)
		if ext == "" {
			continue
		}

		artifactType := strings.TrimPrefix(ext, ".")
		filePath := filepath.Join(targetDir, fileName)

		artifact := createArtifactFromFile(filePath, groupId, artifactId, version, artifactType)
		artifacts = append(artifacts, artifact)
	}

	// Add POM artifact (from project root, not target)
	pomArtifactName := fmt.Sprintf("%s-%s.pom", artifactId, version)
	pomArtifactPath := filepath.Join(workingDir, "pom.xml")

	if _, err := os.Stat(pomArtifactPath); err == nil {
		artifact := createArtifactFromFile(pomArtifactPath, groupId, artifactId, version, "pom")
		artifact.Name = pomArtifactName
		artifact.Path = fmt.Sprintf("%s/%s/%s/%s", strings.ReplaceAll(groupId, ".", "/"), artifactId, version, pomArtifactName)
		artifacts = append(artifacts, artifact)
	}

	// Add artifacts to the first module (Maven projects typically have one module)
	if len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Artifacts = artifacts
		log.Debug(fmt.Sprintf("Added %d artifacts to build info (main packaging: %s)", len(artifacts), packagingType))
	} else {
		log.Warn("No modules found in build info, cannot add artifacts")
	}

	return nil
}

// getPackagingType extracts packaging type from pom.xml
func getPackagingType(workingDir string) string {
	pomPath := filepath.Join(workingDir, "pom.xml")
	pomData, err := os.ReadFile(pomPath)
	if err != nil {
		return "jar" // Default to jar
	}

	var pom PomProject
	if err := xml.Unmarshal(pomData, &pom); err != nil {
		return "jar"
	}

	if pom.Packaging == "" {
		return "jar" // Maven default
	}

	return pom.Packaging
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
