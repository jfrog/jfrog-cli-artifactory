package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	buildinfoflexpack "github.com/jfrog/build-info-go/flexpack"
	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	artifactoryUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

func ShouldRunNative(configPath string) bool {
	return buildinfoflexpack.IsFlexPackEnabled() && configPath == ""
}

func CreateDownloadConfiguration(c *components.Context) (downloadConfiguration *artifactoryUtils.DownloadConfiguration, err error) {
	downloadConfiguration = new(artifactoryUtils.DownloadConfiguration)
	downloadConfiguration.MinSplitSize, err = getMinSplit(c, flagkit.DownloadMinSplitKb)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.SplitCount, err = getSplitCount(c, flagkit.DownloadSplitCount, flagkit.DownloadMaxSplitCount)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.Threads, err = common.GetThreadsCount(c)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.SkipChecksum = c.GetBoolFlagValue("skip-checksum")
	downloadConfiguration.Symlink = true
	return
}

func getMinSplit(c *components.Context, defaultMinSplit int64) (minSplitSize int64, err error) {
	minSplitSize = defaultMinSplit
	if c.GetStringFlagValue(flagkit.MinSplit) != "" {
		minSplitSize, err = strconv.ParseInt(c.GetStringFlagValue(flagkit.MinSplit), 10, 64)
		if err != nil {
			err = errors.New("The '--min-split' option should have a numeric value. " + common.GetDocumentationMessage())
			return 0, err
		}
	}
	return minSplitSize, nil
}

func getSplitCount(c *components.Context, defaultSplitCount, maxSplitCount int) (splitCount int, err error) {
	splitCount = defaultSplitCount
	err = nil
	if c.GetStringFlagValue("split-count") != "" {
		splitCount, err = strconv.Atoi(c.GetStringFlagValue("split-count"))
		if err != nil {
			err = errors.New("The '--split-count' option should have a numeric value. " + common.GetDocumentationMessage())
		}
		if splitCount > maxSplitCount {
			err = errors.New("The '--split-count' option value is limited to a maximum of " + strconv.Itoa(maxSplitCount) + ".")
		}
		if splitCount < 0 {
			err = errors.New("the '--split-count' option cannot have a negative value")
		}
	}
	return
}

func CreateUploadConfiguration(c *components.Context) (uploadConfiguration *artifactoryUtils.UploadConfiguration, err error) {
	uploadConfiguration = new(artifactoryUtils.UploadConfiguration)
	uploadConfiguration.MinSplitSizeMB, err = getMinSplit(c, flagkit.UploadMinSplitMb)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.ChunkSizeMB, err = getUploadChunkSize(c, flagkit.UploadChunkSizeMb)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.SplitCount, err = getSplitCount(c, flagkit.UploadSplitCount, flagkit.UploadMaxSplitCount)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.Threads, err = common.GetThreadsCount(c)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.Deb, err = getDebFlag(c)
	if err != nil {
		return
	}
	return
}

func getUploadChunkSize(c *components.Context, defaultChunkSize int64) (chunkSize int64, err error) {
	chunkSize = defaultChunkSize
	if c.GetStringFlagValue(flagkit.ChunkSize) != "" {
		chunkSize, err = strconv.ParseInt(c.GetStringFlagValue(flagkit.ChunkSize), 10, 64)
		if err != nil {
			err = fmt.Errorf("the '--%s' option should have a numeric value. %s", flagkit.ChunkSize, common.GetDocumentationMessage())
			return 0, err
		}
	}

	return chunkSize, nil
}

func getDebFlag(c *components.Context) (deb string, err error) {
	deb = c.GetStringFlagValue("deb")
	slashesCount := strings.Count(deb, "/") - strings.Count(deb, "\\/")
	if deb != "" && slashesCount != 2 {
		return "", errors.New("the --deb option should be in the form of distribution/component/architecture")
	}
	return deb, nil
}

// GetDependenciesFromLatestBuild fetches all dependencies from the latest build info
// stored in Artifactory for the given build name. Returns a map keyed by dependency ID (name:version).
// projectKey should match the project used when publishing the build (e.g. from buildConfig.GetProject()).
func GetDependenciesFromLatestBuild(servicesManager artifactory.ArtifactoryServicesManager, buildName, projectKey string) (map[string]*entities.Dependency, error) {
	buildDependencies := make(map[string]*entities.Dependency)
	previousBuild, found, err := servicesManager.GetBuildInfo(services.BuildInfoParams{BuildName: buildName, BuildNumber: servicesUtils.LatestBuildNumberKey, ProjectKey: projectKey})
	if err != nil || !found {
		return buildDependencies, err
	}
	for _, module := range previousBuild.BuildInfo.Modules {
		for _, dep := range module.Dependencies {
			buildDependencies[dep.Id] = &entities.Dependency{
				Id:   dep.Id,
				Type: dep.Type,
				Checksum: entities.Checksum{
					Md5:    dep.Md5,
					Sha1:   dep.Sha1,
					Sha256: dep.Sha256,
				},
			}
		}
	}
	return buildDependencies, nil
}

// DependenciesToChecksumMap converts a dependency map to a checksum map,
// keeping only entries with non-empty checksums.
func DependenciesToChecksumMap(deps map[string]*entities.Dependency) map[string]entities.Checksum {
	checksumMap := make(map[string]entities.Checksum, len(deps))
	for id, dep := range deps {
		if !dep.IsEmpty() {
			checksumMap[id] = dep.Checksum
		}
	}
	return checksumMap
}

// WriteResultItemsToFile writes ResultItem records to a temp file via ContentWriter
// and returns the file path. Callers can create a ContentReader from this path
// for use with SetProps or other batch operations.
func WriteResultItemsToFile(items []servicesUtils.ResultItem) (filePath string, err error) {
	writer, err := content.NewContentWriter("results", true, false)
	if err != nil {
		return
	}
	defer ioutils.Close(writer, &err)
	for _, item := range items {
		writer.Write(item)
	}
	filePath = writer.GetFilePath()
	return
}

// ExecuteAqlQuery executes an AQL query and parses the JSON response
func ExecuteAqlQuery(serviceManager artifactory.ArtifactoryServicesManager, aqlQuery string) ([]servicesUtils.ResultItem, error) {
	reader, err := serviceManager.Aql(aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute AQL query: %w", err)
	}
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()
	aqlResults, err := io.ReadAll(reader)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	parsedResult := new(servicesUtils.AqlSearchResult)
	if err = json.Unmarshal(aqlResults, parsedResult); err != nil {
		return nil, errorutils.CheckError(err)
	}
	if parsedResult.Results == nil {
		return nil, nil
	}
	return parsedResult.Results, nil
}
