package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	coreUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
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
	repo               string
}

// Run executes the download command to fetch a model or dataset from HuggingFace Hub
func (hfd *HuggingFaceDownload) Run() error {
	if hfd.repoId == "" {
		return errorutils.CheckErrorf("repo_id cannot be empty")
	}
	serviceManager, err := coreUtils.CreateServiceManager(hfd.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}
	repo, err := handleRepositoryResolution(serviceManager, hfd.serverDetails, "download")
	if err != nil {
		return err
	}
	hfd.repo = repo
	pythonPath, err := GetPythonPath()
	if err != nil {
		return err
	}
	scriptDir, err := extractPythonScripts()
	if err != nil {
		return err
	}
	defer func(path string) {
		removeErr := os.RemoveAll(path)
		if removeErr != nil {
			log.Error(removeErr)
			return
		}
	}(scriptDir)
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
		return hfd.CollectDependenciesForBuildInfo(result.ModelPath)
	}
	return nil
}

func (hfd *HuggingFaceDownload) CollectDependenciesForBuildInfo(localPath string) error {
	ctx, err := GetBuildInfoContext(hfd.buildConfiguration, hfd.name)
	if err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}
	dependencies, err := hfd.GetDependencies(localPath)
	if err != nil {
		return errorutils.CheckError(err)
	}
	if len(dependencies) == 0 {
		return nil
	}
	moduleId := hfd.buildConfiguration.GetModule()
	if moduleId == "" {
		moduleId = fmt.Sprintf("%s-%s", ctx.BuildName, ctx.BuildNumber)
	}
	module := entities.Module{
		Type: entities.ModuleType("huggingfaceml"),
		Id:   moduleId,
	}
	module.Dependencies = dependencies
	removeDuplicateDependencies(&module)
	ctx.BuildInfo.Modules = append(ctx.BuildInfo.Modules, module)
	return SaveBuildInfo(ctx)
}

// GetDependencies walks the local downloaded directory and computes checksums for each file.
func (hfd *HuggingFaceDownload) GetDependencies(localPath string) ([]entities.Dependency, error) {
	var dependencies []entities.Dependency
	err := filepath.WalkDir(localPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		details, err := fileutils.GetFileDetails(path, true)
		if err != nil {
			return fmt.Errorf("failed to compute checksums for %s: %w", path, err)
		}
		fileName := d.Name()
		dependencies = append(dependencies, entities.Dependency{
			Id:         fileName,
			Type:       strings.TrimPrefix(filepath.Ext(fileName), "."),
			Repository: hfd.repo,
			Checksum: entities.Checksum{
				Md5:    details.Checksum.Md5,
				Sha1:   details.Checksum.Sha1,
				Sha256: details.Checksum.Sha256,
			},
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk downloaded directory %s: %w", localPath, err)
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

// SetCommandName sets the command name for the download command
func (hfd *HuggingFaceDownload) SetCommandName(commandName string) *HuggingFaceDownload {
	hfd.name = commandName
	return hfd
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
