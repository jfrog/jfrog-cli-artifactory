package conan

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ConanPackageInfo holds parsed Conan package reference information.
// Supports both Conan 2.x (name/version) and 1.x (name/version@user/channel) formats.
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

	return ac.searchArtifacts(buildArtifactQuery(ac.targetRepo, pkgInfo))
}

// CollectArtifactsForPath collects artifacts from a specific path.
// Used to collect only artifacts that were uploaded in the current build.
// The path should be exact (e.g., "_/multideps/1.0.0/_/revision/export")
func (ac *ArtifactCollector) CollectArtifactsForPath(exactPath string) ([]entities.Artifact, error) {
	if ac.serverDetails == nil {
		return nil, fmt.Errorf("server details not initialized")
	}

	// Use exact path match - artifacts are directly in the path, not subfolders
	query := fmt.Sprintf(`{"repo": "%s", "path": "%s"}`, ac.targetRepo, exactPath)
	return ac.searchArtifacts(query)
}

// searchArtifacts executes an AQL query and returns matching artifacts.
func (ac *ArtifactCollector) searchArtifacts(aqlQuery string) ([]entities.Artifact, error) {
	servicesManager, err := utils.CreateServiceManager(ac.serverDetails, -1, 0, false)
	if err != nil {
		return nil, fmt.Errorf("create services manager: %w", err)
	}

	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: aqlQuery},
		},
	}

	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}
	defer closeReader(reader)

	return parseSearchResults(reader), nil
}

// parseSearchResults converts AQL search results to artifacts.
func parseSearchResults(reader *content.ContentReader) []entities.Artifact {
	var artifacts []entities.Artifact

	for item := new(specutils.ResultItem); reader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
		artifact := entities.Artifact{
			Name: item.Name,
			Path: item.Path,
			Checksum: entities.Checksum{
				Sha1:   item.Actual_Sha1,
				Sha256: item.Sha256,
				Md5:    item.Actual_Md5,
			},
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts
}

// ParsePackageReference parses a Conan package reference string into structured info.
// Supports both formats:
//   - Conan 2.x: name/version (e.g., "zlib/1.2.13")
//   - Conan 1.x: name/version@user/channel (e.g., "zlib/1.2.13@_/_")
func ParsePackageReference(ref string) (*ConanPackageInfo, error) {
	ref = strings.TrimSpace(ref)

	// Check for @user/channel format (Conan 1.x style)
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
// Conan stores artifacts in different path formats depending on version:
//   - Conan 2.x: _/name/version/_/revision/...
//   - Conan 1.x: user/name/version/channel/revision/...
func buildArtifactQuery(repo string, pkg *ConanPackageInfo) string {
	if pkg.User == "_" && pkg.Channel == "_" {
		return fmt.Sprintf(`{"repo": "%s", "path": {"$match": "_/%s/%s/_/*"}}`,
			repo, pkg.Name, pkg.Version)
	}
	return fmt.Sprintf(`{"repo": "%s", "path": {"$match": "%s/%s/%s/%s/*"}}`,
		repo, pkg.User, pkg.Name, pkg.Version, pkg.Channel)
}

// BuildPropertySetter sets build properties on Conan artifacts in Artifactory.
// This is required to link artifacts to build info in Artifactory UI.
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

// SetProperties sets build properties on the given artifacts in a single batch operation.
// This uses the same approach as Docker - writing all items to a temp file and making
// one SetProps call, which is much more efficient than individual calls per artifact.
func (bps *BuildPropertySetter) SetProperties(artifacts []entities.Artifact) error {
	if len(artifacts) == 0 || bps.serverDetails == nil {
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(bps.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create services manager: %w", err)
	}

	// Convert artifacts to ResultItem format for batch processing
	resultItems := bps.convertToResultItems(artifacts)
	if len(resultItems) == 0 {
		return nil
	}

	// Write all items to a temp file (like Docker does)
	pathToFile, err := bps.writeItemsToFile(resultItems)
	if err != nil {
		return fmt.Errorf("write items to file: %w", err)
	}

	// Create reader and set properties in one batch call
	reader := content.NewContentReader(pathToFile, content.DefaultKey)
	defer closeReader(reader)

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := bps.formatBuildProperties(timestamp)

	_, err = servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true, IsRecursive: true})
	if err != nil {
		return fmt.Errorf("set properties: %w", err)
	}

	log.Info(fmt.Sprintf("Set build properties on %d Conan artifacts (batch)", len(artifacts)))
	return nil
}

// convertToResultItems converts build-info artifacts to ResultItem format for SetProps.
func (bps *BuildPropertySetter) convertToResultItems(artifacts []entities.Artifact) []specutils.ResultItem {
	var items []specutils.ResultItem
	for _, artifact := range artifacts {
		items = append(items, specutils.ResultItem{
			Repo:        bps.targetRepo,
			Path:        artifact.Path,
			Name:        artifact.Name,
			Actual_Sha1: artifact.Sha1,
			Actual_Md5:  artifact.Md5,
			Sha256:      artifact.Sha256,
		})
	}
	return items
}

// writeItemsToFile writes result items to a temp file for batch processing.
func (bps *BuildPropertySetter) writeItemsToFile(items []specutils.ResultItem) (string, error) {
	writer, err := content.NewContentWriter("results", true, false)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := writer.Close(); closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close writer: %s", closeErr))
		}
	}()

	for _, item := range items {
		writer.Write(item)
	}
	return writer.GetFilePath(), nil
}

// formatBuildProperties creates the build properties string.
// Only includes build.name, build.number, build.timestamp (and optional build.project).
func (bps *BuildPropertySetter) formatBuildProperties(timestamp string) string {
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s",
		bps.buildName, bps.buildNumber, timestamp)

	if bps.projectKey != "" {
		props += fmt.Sprintf(";build.project=%s", bps.projectKey)
	}

	return props
}

// closeReader safely closes a content reader.
func closeReader(reader *content.ContentReader) {
	if reader != nil {
		if err := reader.Close(); err != nil {
			log.Debug(fmt.Sprintf("Failed to close reader: %s", err))
		}
	}
}
