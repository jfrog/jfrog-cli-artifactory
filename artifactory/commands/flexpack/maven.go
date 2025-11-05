package flexpack

import (
	"encoding/xml"
	"fmt"
	"net/url"
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

// SettingsXml represents Maven settings.xml structure
type SettingsXml struct {
	XMLName        xml.Name          `xml:"settings"`
	ActiveProfiles []string          `xml:"activeProfiles>activeProfile"`
	Profiles       []SettingsProfile `xml:"profiles>profile"`
}

type SettingsProfile struct {
	Id                              string       `xml:"id"`
	AltDeploymentRepository         string       `xml:"properties>altDeploymentRepository"`
	AltReleaseDeploymentRepository  string       `xml:"properties>altReleaseDeploymentRepository"`
	AltSnapshotDeploymentRepository string       `xml:"properties>altSnapshotDeploymentRepository"`
	Repositories                    []Repository `xml:"repositories>repository"`
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

	// Filter to only artifacts modified in the last 2 minutes (just deployed)
	cutoffTime := time.Now().Add(-2 * time.Minute)
	var recentArtifacts []specutils.ResultItem

	for item := new(specutils.ResultItem); searchReader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
		// Parse the modified time
		modTime, err := time.Parse("2006-01-02T15:04:05.999Z", item.Modified)
		if err != nil {
			log.Debug("Could not parse modified time for " + item.Name + ": " + err.Error())
			continue
		}

		// Only include artifacts modified after cutoff
		if modTime.After(cutoffTime) {
			recentArtifacts = append(recentArtifacts, *item)
		}
	}

	if len(recentArtifacts) == 0 {
		log.Warn("No recently deployed artifacts found")
		return nil
	}

	// Create build properties in the same format as NPM/traditional implementations
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10) // Unix milliseconds like NPM
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if projectKey := buildArgs.GetProject(); projectKey != "" {
		buildProps += fmt.Sprintf(";build.project=%s", projectKey)
	}

	// Set properties on each recent artifact individually
	for _, artifact := range recentArtifacts {
		// ResultItem has Repo, Path, and Name fields already separated
		// Use AQL to find the exact artifact
		aqlPattern := fmt.Sprintf(`{"repo":"%s","path":"%s","name":"%s"}`,
			targetRepo, artifact.Path, artifact.Name)

		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Aql: specutils.Aql{
					ItemsFind: aqlPattern,
				},
			},
		}

		reader, err := servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to search for artifact %s: %s", artifact.Name, err))
			continue
		}

		propsParams := services.PropsParams{
			Reader: reader,
			Props:  buildProps,
		}

		_, err = servicesManager.SetProps(propsParams)
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close reader for %s: %s", artifact.Name, closeErr))
		}
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to set properties on %s: %s", artifact.Name, err))
		}
	}

	log.Info("Successfully set build properties on deployed Maven artifacts")
	return nil
}

// getSettingsXmlPath finds the Maven settings.xml file
func getSettingsXmlPath() string {
	// Check if -s or --settings flag was used
	args := os.Args
	for i, arg := range args {
		if (arg == "-s" || arg == "--settings") && i+1 < len(args) {
			return args[i+1]
		}
	}

	// Default location: ~/.m2/settings.xml
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".m2", "settings.xml")
}

// parseSettingsXml reads and parses Maven settings.xml
func parseSettingsXml(settingsPath string) (*SettingsXml, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}

	var settings SettingsXml
	if err := xml.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

// isSnapshotVersion checks if a Maven version is a SNAPSHOT
func isSnapshotVersion(version string) bool {
	return strings.HasSuffix(strings.TrimSpace(version), "-SNAPSHOT")
}

// extractRepoFromAltProperty parses the alt*DeploymentRepository format: "id::layout::url"
func extractRepoFromAltProperty(altRepo string) (string, error) {
	parts := strings.Split(altRepo, "::")
	if len(parts) >= 3 {
		repoUrl := parts[2]
		return extractRepoKeyFromUrl(repoUrl)
	}
	return "", fmt.Errorf("invalid alt deployment repository format: %s", altRepo)
}

// getRepositoryFromSettings extracts deployment repository from settings.xml
// based on whether the project is a SNAPSHOT or RELEASE version
func getRepositoryFromSettings(isSnapshot bool) (string, error) {
	settingsPath := getSettingsXmlPath()
	if settingsPath == "" {
		return "", fmt.Errorf("could not determine settings.xml path")
	}

	settings, err := parseSettingsXml(settingsPath)
	if err != nil {
		return "", err
	}

	// Check active profiles for alt*DeploymentRepository with Maven's actual precedence
	// Maven prioritizes SPECIFIC (altSnapshot/altRelease) over GENERAL (altDeployment)
	for _, profileId := range settings.ActiveProfiles {
		for _, profile := range settings.Profiles {
			if profile.Id == profileId {
				// Priority 1: altSnapshotDeploymentRepository or altReleaseDeploymentRepository (SPECIFIC wins)
				if isSnapshot && profile.AltSnapshotDeploymentRepository != "" {
					log.Debug("Found altSnapshotDeploymentRepository in settings.xml (specific for SNAPSHOT)")
					return extractRepoFromAltProperty(profile.AltSnapshotDeploymentRepository)
				}

				if !isSnapshot && profile.AltReleaseDeploymentRepository != "" {
					log.Debug("Found altReleaseDeploymentRepository in settings.xml (specific for RELEASE)")
					return extractRepoFromAltProperty(profile.AltReleaseDeploymentRepository)
				}

				// Priority 2: altDeploymentRepository (GENERAL fallback)
				if profile.AltDeploymentRepository != "" {
					log.Debug("Found altDeploymentRepository in settings.xml (general fallback)")
					return extractRepoFromAltProperty(profile.AltDeploymentRepository)
				}
			}
		}
	}

	return "", fmt.Errorf("no deployment repository found in settings.xml")
}

// extractRepoKeyFromUrl extracts repository key from Artifactory URL using proper URL parsing
func extractRepoKeyFromUrl(repoUrl string) (string, error) {
	repoUrl = strings.TrimSpace(repoUrl)

	// Parse the URL
	u, err := url.Parse(repoUrl)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	// Split path into segments, removing empty strings
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")

	// Handle /api/maven/REPO-KEY format
	// Path: /artifactory/api/maven/REPO-KEY
	// Segments: [artifactory, api, maven, REPO-KEY]
	if len(segments) >= 4 && segments[len(segments)-3] == "api" && segments[len(segments)-2] == "maven" {
		repoKey := segments[len(segments)-1]
		if repoKey != "" {
			return repoKey, nil
		}
	}

	// Standard format: /artifactory/REPO-KEY
	// Segments: [artifactory, REPO-KEY]
	// The last segment is the repository key
	if len(segments) >= 2 {
		repoKey := segments[len(segments)-1]
		if repoKey != "" {
			return repoKey, nil
		}
	}

	return "", fmt.Errorf("unable to extract repository key from URL: %s (check repository URL format)", repoUrl)
}

// getMavenDeployRepository determines where Maven deployed artifacts
// by parsing pom.xml distributionManagement, with fallback to settings.xml
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

	// Determine project version to know if it's SNAPSHOT or RELEASE
	version := pom.Version
	if version == "" && pom.Parent.Version != "" {
		version = pom.Parent.Version
	}
	isSnapshot := isSnapshotVersion(version)
	log.Debug(fmt.Sprintf("Project version: %s, isSnapshot: %v", version, isSnapshot))

	// Priority 1: Check settings.xml (Maven standard precedence)
	// settings.xml alt*DeploymentRepository overrides pom.xml in Maven
	repoKey, err := getRepositoryFromSettings(isSnapshot)
	if err == nil {
		log.Debug("Found deploy repository from settings.xml (overriding pom.xml): " + repoKey)
		return repoKey, nil
	}

	// Priority 2: Check pom.xml distributionManagement
	var repoUrl string
	switch {
	case isSnapshot && pom.DistributionManagement.SnapshotRepository.URL != "":
		repoUrl = pom.DistributionManagement.SnapshotRepository.URL
		log.Debug("Using snapshotRepository from pom.xml")
	case !isSnapshot && pom.DistributionManagement.Repository.URL != "":
		repoUrl = pom.DistributionManagement.Repository.URL
		log.Debug("Using repository from pom.xml")
	case pom.DistributionManagement.Repository.URL != "":
		// Fallback: use release repository if snapshot not defined
		repoUrl = pom.DistributionManagement.Repository.URL
		log.Debug("Using repository (fallback) from pom.xml")
	}

	if repoUrl != "" {
		repoKey, err := extractRepoKeyFromUrl(repoUrl)
		if err == nil {
			log.Debug("Found deploy repository from pom.xml: " + repoKey)
			return repoKey, nil
		}
		log.Debug("Failed to extract repository from pom.xml URL: " + err.Error())
	}

	return "", fmt.Errorf("no deployment repository found in settings.xml or pom.xml")
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

	// Only include the main artifact that matches the packaging type
	// This follows traditional Maven behavior where intermediate build artifacts (e.g., .jar in WAR projects) are excluded
	mainArtifactName := fmt.Sprintf("%s-%s.%s", artifactId, version, packagingType)
	mainArtifactPath := filepath.Join(targetDir, mainArtifactName)

	if _, err := os.Stat(mainArtifactPath); err == nil {
		artifact := createArtifactFromFile(mainArtifactPath, groupId, artifactId, version, packagingType)
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
