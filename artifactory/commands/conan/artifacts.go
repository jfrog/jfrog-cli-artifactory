package conan

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ConanPackageInfo holds parsed Conan package reference information.
type ConanPackageInfo struct {
	Name    string
	Version string
	User    string
	Channel string
}

// ArtifactCollector collects Conan artifacts from Artifactory.
type ArtifactCollector struct {
	serverDetails *config.ServerDetails
	targetRepo    string
}

// NewArtifactCollector creates a new artifact collector.
func NewArtifactCollector(serverDetails *config.ServerDetails, targetRepo string) *ArtifactCollector {
	return &ArtifactCollector{
		serverDetails: serverDetails,
		targetRepo:    targetRepo,
	}
}

// CollectArtifacts searches Artifactory for Conan artifacts matching the package reference.
func (ac *ArtifactCollector) CollectArtifacts(packageRef string) ([]entities.Artifact, error) {
	if ac.serverDetails == nil {
		return nil, fmt.Errorf("server details not initialized")
	}

	pkgInfo, err := ParsePackageReference(packageRef)
	if err != nil {
		return nil, err
	}

	servicesManager, err := utils.CreateServiceManager(ac.serverDetails, -1, 0, false)
	if err != nil {
		return nil, fmt.Errorf("create services manager: %w", err)
	}

	return ac.searchArtifacts(servicesManager, pkgInfo)
}

// searchArtifacts searches for Conan artifacts in Artifactory.
func (ac *ArtifactCollector) searchArtifacts(manager artifactory.ArtifactoryServicesManager, pkgInfo *ConanPackageInfo) ([]entities.Artifact, error) {
	searchQuery := buildArtifactQuery(ac.targetRepo, pkgInfo)
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}

	reader, err := manager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}
	defer closeReader(reader)

	return parseSearchResults(reader)
}

// parseSearchResults converts AQL search results to artifacts.
func parseSearchResults(reader *content.ContentReader) ([]entities.Artifact, error) {
	var artifacts []entities.Artifact

	for item := new(specutils.ResultItem); reader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
		artifact := entities.Artifact{
			Name: item.Name,
			Path: item.Path,
			Type: classifyArtifactType(item.Name),
			Checksum: entities.Checksum{
				Sha1:   item.Actual_Sha1,
				Sha256: item.Sha256,
				Md5:    item.Actual_Md5,
			},
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

// classifyArtifactType determines the artifact type based on filename.
func classifyArtifactType(name string) string {
	switch {
	case name == "conanfile.py":
		return "conan-recipe"
	case name == "conanmanifest.txt":
		return "conan-manifest"
	case name == "conaninfo.txt":
		return "conan-info"
	case strings.HasSuffix(name, ".tgz"):
		return "conan-package"
	default:
		return "conan-artifact"
	}
}

// ParsePackageReference parses a Conan package reference string.
// Supports both formats: name/version and name/version@user/channel
func ParsePackageReference(ref string) (*ConanPackageInfo, error) {
	// Remove any trailing whitespace or package ID
	ref = strings.TrimSpace(ref)
	
	// Check for @user/channel format
	if idx := strings.Index(ref, "@"); idx != -1 {
		nameVersion := ref[:idx]
		userChannel := ref[idx+1:]

		nameParts := strings.SplitN(nameVersion, "/", 2)
		channelParts := strings.SplitN(userChannel, "/", 2)

		if len(nameParts) != 2 || len(channelParts) != 2 {
			return nil, fmt.Errorf("invalid package reference: %s", ref)
		}

		return &ConanPackageInfo{
			Name:    nameParts[0],
			Version: nameParts[1],
			User:    channelParts[0],
			Channel: channelParts[1],
		}, nil
	}

	// Simple name/version format (Conan 2.x style)
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid package reference: %s", ref)
	}

	return &ConanPackageInfo{
		Name:    parts[0],
		Version: parts[1],
		User:    "_",
		Channel: "_",
	}, nil
}

// buildArtifactQuery creates an AQL query for Conan artifacts.
func buildArtifactQuery(repo string, pkg *ConanPackageInfo) string {
	// Conan 2.x uses _/name/version/_ path format
	if pkg.User == "_" && pkg.Channel == "_" {
		return fmt.Sprintf(`{"repo": "%s", "path": {"$match": "_/%s/%s/_/*"}}`,
			repo, pkg.Name, pkg.Version)
	}
	// Conan 1.x uses user/name/version/channel path format
	return fmt.Sprintf(`{"repo": "%s", "path": {"$match": "%s/%s/%s/%s/*"}}`,
		repo, pkg.User, pkg.Name, pkg.Version, pkg.Channel)
}

// BuildPropertySetter sets build properties on Conan artifacts.
type BuildPropertySetter struct {
	serverDetails *config.ServerDetails
	targetRepo    string
	buildName     string
	buildNumber   string
	projectKey    string
}

// NewBuildPropertySetter creates a new build property setter.
func NewBuildPropertySetter(serverDetails *config.ServerDetails, targetRepo, buildName, buildNumber, projectKey string) *BuildPropertySetter {
	return &BuildPropertySetter{
		serverDetails: serverDetails,
		targetRepo:    targetRepo,
		buildName:     buildName,
		buildNumber:   buildNumber,
		projectKey:    projectKey,
	}
}

// SetProperties sets build properties on the given artifacts.
func (bps *BuildPropertySetter) SetProperties(artifacts []entities.Artifact) error {
	if len(artifacts) == 0 || bps.serverDetails == nil {
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(bps.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create services manager: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := bps.formatBuildProperties(timestamp)

	successCount := 0
	for _, artifact := range artifacts {
		if err := bps.setPropertiesOnArtifact(servicesManager, artifact, props); err != nil {
			log.Warn(fmt.Sprintf("Failed to set properties on %s: %s", artifact.Name, err.Error()))
			continue
		}
		successCount++
	}

	log.Info(fmt.Sprintf("Successfully set build properties on %d Conan artifacts", successCount))
	return nil
}

// formatBuildProperties creates the build properties string.
func (bps *BuildPropertySetter) formatBuildProperties(timestamp string) string {
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s",
		bps.buildName, bps.buildNumber, timestamp)

	if bps.projectKey != "" {
		props += fmt.Sprintf(";build.project=%s", bps.projectKey)
	}

	return props
}

// setPropertiesOnArtifact sets properties on a single artifact.
func (bps *BuildPropertySetter) setPropertiesOnArtifact(manager artifactory.ArtifactoryServicesManager, artifact entities.Artifact, props string) error {
	artifactPath := fmt.Sprintf("%s/%s/%s", bps.targetRepo, artifact.Path, artifact.Name)

	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Pattern: artifactPath,
		},
	}

	reader, err := manager.SearchFiles(searchParams)
	if err != nil {
		return fmt.Errorf("search artifact: %w", err)
	}
	defer closeReader(reader)

	propsParams := services.PropsParams{
		Reader: reader,
		Props:  props,
	}

	_, err = manager.SetProps(propsParams)
	return err
}

// closeReader safely closes a content reader.
func closeReader(reader *content.ContentReader) {
	if reader != nil {
		if err := reader.Close(); err != nil {
			log.Debug(fmt.Sprintf("Failed to close reader: %s", err))
		}
	}
}
