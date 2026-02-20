package cli

import (
	"fmt"
	"os/exec"
	"sort"

	"github.com/jfrog/build-info-go/entities"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// Response represents the result of a HuggingFace operation
type Response struct {
	Success   bool   `json:"success"`
	ModelPath string `json:"model_path,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HuggingFaceDownload represents a command to download models or datasets from HuggingFace Hub
type HuggingFaceDownload struct {
	name               string
	repoId             string
	revision           string
	repoType           string
	etagTimeout        int
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
}

// Run executes the download command to fetch a model or dataset from HuggingFace Hub
func (hfd *HuggingFaceDownload) Run() error {
	if hfd.repoId == "" {
		return errorutils.CheckErrorf("repo_id cannot be empty")
	}

	// Find huggingface-cli or hf command (may fall back to Python module mode)
	hfCliPath, prefixArgs, err := GetHuggingFaceCliPath()
	if err != nil {
		return err
	}

	// Build huggingface-cli download command arguments
	repoType := hfd.repoType
	if repoType == "" {
		repoType = "model"
	}

	var cmdArgs []string
	cmdArgs = append(cmdArgs, prefixArgs...)
	cmdArgs = append(cmdArgs, "download", hfd.repoId, "--repo-type", repoType)

	if hfd.revision != "" {
		cmdArgs = append(cmdArgs, "--revision", hfd.revision)
	}

	if hfd.etagTimeout > 0 {
		cmdArgs = append(cmdArgs, "--etag-timeout", fmt.Sprintf("%d", hfd.etagTimeout))
	}

	log.Debug("Executing: ", hfCliPath, " ", cmdArgs)

	cmd := exec.Command(hfCliPath, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorutils.CheckErrorf("huggingface-cli download failed: %w\nOutput: %s", err, string(output))
	}

	log.Info("Downloaded successfully: ", hfd.repoId)

	if hfd.buildConfiguration != nil {
		return hfd.CollectDependenciesForBuildInfo()
	}
	return nil
}

func (hfd *HuggingFaceDownload) CollectDependenciesForBuildInfo() error {
	ctx, err := GetBuildInfoContext(hfd.buildConfiguration, hfd.name)
	if err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}
	dependencies, err := hfd.GetDependencies()
	if err != nil {
		return errorutils.CheckError(err)
	}
	if len(dependencies) == 0 {
		return nil
	}
	// Add module and set dependencies before saving
	if len(ctx.BuildInfo.Modules) == 0 {
		ctx.BuildInfo.Modules = append(ctx.BuildInfo.Modules, entities.Module{
			Type: entities.ModuleType(hfd.repoType),
			Id:   hfd.repoId,
		})
	}
	ctx.BuildInfo.Modules[0].Dependencies = dependencies
	return SaveBuildInfo(ctx)
}

// GetDependencies returns HuggingFace model/dataset files in JFrog Artifactory
func (hfd *HuggingFaceDownload) GetDependencies() ([]entities.Dependency, error) {
	serviceManager, err := utils.CreateServiceManager(hfd.serverDetails, -1, 0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create services manager: %w", err)
	}
	repoKey, err := GetRepoKeyFromHFEndpoint()
	if err != nil {
		return nil, err
	}
	repoTypePath := hfd.repoType + "s"
	if hfd.repoType == "" {
		repoTypePath = "models"
	}
	revisionPattern := hfd.revision
	var multipleDirsInSearchResults = false
	if !HasTimestamp(hfd.revision) {
		revisionPattern = hfd.revision + "_*"
		multipleDirsInSearchResults = true
	}
	aqlQuery := fmt.Sprintf(`{"repo": "%s", "path": {"$match": "%s/%s/%s/*"}}`,
		repoKey,
		repoTypePath,
		hfd.repoId,
		revisionPattern,
	)
	searchParams := services.SearchParams{
		CommonParams: &servicesUtils.CommonParams{
			Aql: servicesUtils.Aql{ItemsFind: aqlQuery},
		},
	}
	reader, err := serviceManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to search for HuggingFace artifacts: %w", err)
	}
	defer func(reader *content.ContentReader) {
		err := reader.Close()
		if err != nil {
			log.Error(err)
		}
	}(reader)
	var results []servicesUtils.ResultItem
	for item := new(servicesUtils.ResultItem); reader.NextRecord(item) == nil; item = new(servicesUtils.ResultItem) {
		results = append(results, *item)
	}
	if len(results) == 0 {
		return nil, nil
	}
	if multipleDirsInSearchResults {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Path > results[j].Path
		})
	}
	var latestCreatedDir string
	var dependencies []entities.Dependency
	for index, resultItem := range results {
		if index == 0 {
			latestCreatedDir = resultItem.Path
		}
		dependencies = append(dependencies, entities.Dependency{
			Id:         resultItem.Name,
			Type:       resultItem.Type,
			Repository: resultItem.Repo,
			Checksum: entities.Checksum{
				Sha1:   resultItem.Actual_Sha1,
				Md5:    resultItem.Actual_Md5,
				Sha256: resultItem.Sha256,
			},
		})
		if latestCreatedDir != resultItem.Path {
			break
		}
	}
	return dependencies, nil
}

// ServerDetails returns the server details configuration for the command
func (hfd *HuggingFaceDownload) ServerDetails() (*config.ServerDetails, error) {
	return hfd.serverDetails, nil
}

// CommandName returns the name of the command
func (hfd *HuggingFaceDownload) CommandName() string {
	return hfd.name
}

// NewHuggingFaceDownload creates a new instance of HFDownloadCmd
func NewHuggingFaceDownload() *HuggingFaceDownload {
	return &HuggingFaceDownload{}
}

// SetRepoId sets the repository ID for the download command
func (hfd *HuggingFaceDownload) SetRepoId(repoId string) *HuggingFaceDownload {
	hfd.repoId = repoId
	return hfd
}

// SetRevision sets the revision (branch, tag, or commit) for the download command
func (hfd *HuggingFaceDownload) SetRevision(revision string) *HuggingFaceDownload {
	hfd.revision = revision
	return hfd
}

// SetServerDetails sets the revision (branch, tag, or commit) for the upload command
func (hfd *HuggingFaceDownload) SetServerDetails(serverDetails *config.ServerDetails) *HuggingFaceDownload {
	hfd.serverDetails = serverDetails
	return hfd
}

// SetBuildConfiguration sets the build configuration
func (hfd *HuggingFaceDownload) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *HuggingFaceDownload {
	hfd.buildConfiguration = buildConfiguration
	return hfd
}

// SetRepoType sets the repository type (model, dataset, or space) for the download command
func (hfd *HuggingFaceDownload) SetRepoType(repoType string) *HuggingFaceDownload {
	hfd.repoType = repoType
	return hfd
}

// SetEtagTimeout sets the ETag validation timeout in seconds for the download command
func (hfd *HuggingFaceDownload) SetEtagTimeout(etageTimeout int) *HuggingFaceDownload {
	hfd.etagTimeout = etageTimeout
	return hfd
}
