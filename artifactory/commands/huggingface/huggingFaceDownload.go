package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
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
	pythonPath, err := GetPythonPath()
	if err != nil {
		return err
	}
	scriptDir, err := getHuggingFaceScriptPath("huggingface_download.py")
	if err != nil {
		return errorutils.CheckError(err)
	}
	args := map[string]interface{}{
		"repo_id": hfd.repoId,
	}
	if hfd.revision != "" {
		args["revision"] = hfd.revision
	}
	if hfd.repoType != "" {
		args["repo_type"] = hfd.repoType
	}
	if hfd.etagTimeout > 0 {
		args["etag_timeout"] = hfd.etagTimeout
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return errorutils.CheckErrorf("failed to marshal arguments to JSON: %w", err)
	}
	pythonCmd := BuildPythonDownloadCmd(string(argsJSON))
	log.Debug("Executing Python function to download ", args["repo_type"], ": ", hfd.repoId)
	cmd := exec.Command(pythonPath, "-u", "-c", pythonCmd)
	cmd.Dir = scriptDir
	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = io.MultiWriter(os.Stderr)
	err = cmd.Run()
	output := stdoutBuf.Bytes()
	if len(output) == 0 {
		if err != nil {
			return errorutils.CheckErrorf("Python script produced no output and exited with error: %w", err)
		}
		return errorutils.CheckErrorf("Python script produced no output. The script may not be executing correctly.")
	}
	var result Response
	if jsonErr := json.Unmarshal(output, &result); jsonErr != nil {
		if err != nil {
			return errorutils.CheckErrorf("failed to execute Python script: %w, output: %s", err, string(output))
		}
		return errorutils.CheckErrorf("failed to parse Python script output: %w, output: %s", jsonErr, string(output))
	}
	if !result.Success {
		return errorutils.CheckErrorf("%s", result.Error)
	}
	if err != nil {
		return errorutils.CheckErrorf("Python script execution failed: %w", err)
	}
	log.Info(fmt.Sprintf("Downloaded successfully to: %s", result.ModelPath))
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
	aqlQuery := fmt.Sprintf(`items.find({"repo":"%s","path":{"$match":"%s/%s/%s/*"}}).include("repo","path","name","actual_sha1","actual_md5","sha256","type")`,
		repoKey,
		repoTypePath,
		hfd.repoId,
		revisionPattern,
	)
	results, err := executeAqlQuery(serviceManager, aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to search for HuggingFace artifacts: %w", err)
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
