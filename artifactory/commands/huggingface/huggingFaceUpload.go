package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// HuggingFaceUpload represents a command to upload models or datasets to HuggingFace Hub
type HuggingFaceUpload struct {
	name               string
	folderPath         string
	repoId             string
	revision           string
	repoType           string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
}

// Run executes the upload command to upload a model or dataset folder to HuggingFace Hub
func (hfu *HuggingFaceUpload) Run() error {
	if hfu.folderPath == "" {
		return errorutils.CheckErrorf("folder_path cannot be empty")
	}
	if hfu.repoId == "" {
		return errorutils.CheckErrorf("repo_id cannot be empty")
	}
	pythonPath, err := GetPythonPath()
	if err != nil {
		return err
	}
	scriptDir, err := getHuggingFaceScriptPath("huggingface_upload.py")
	if err != nil {
		return errorutils.CheckError(err)
	}
	args := map[string]interface{}{
		"folder_path": hfu.folderPath,
		"repo_id":     hfu.repoId,
	}
	if hfu.revision != "" {
		args["revision"] = hfu.revision
	}
	if hfu.repoType != "" {
		args["repo_type"] = hfu.repoType
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return errorutils.CheckErrorf("failed to marshal arguments to JSON: %w", err)
	}
	pythonCmd := BuildPythonUploadCmd(string(argsJSON))
	log.Debug("Executing Python function to upload ", args["repo_type"], ": ", hfu.folderPath, " to ", hfu.repoId)
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
	log.Info(fmt.Sprintf("Uploaded successfully to: %s", hfu.repoId))
	if hfu.buildConfiguration != nil {
		return hfu.CollectArtifactsForBuildInfo()
	}
	return nil
}

func (hfu *HuggingFaceUpload) CollectArtifactsForBuildInfo() error {
	ctx, err := GetBuildInfoContext(hfu.buildConfiguration, hfu.name)
	if err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}
	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", ctx.BuildName, ctx.BuildNumber, timestamp)
	if ctx.Project != "" {
		buildProps += fmt.Sprintf(";build.project=%s", ctx.Project)
	}
	artifacts, err := hfu.GetArtifacts(buildProps)
	if err != nil {
		return errorutils.CheckError(err)
	}
	if len(artifacts) == 0 {
		return nil
	}
	// Add module and set artifacts before saving
	if len(ctx.BuildInfo.Modules) == 0 {
		ctx.BuildInfo.Modules = append(ctx.BuildInfo.Modules, entities.Module{
			Type: entities.ModuleType(hfu.repoType),
			Id:   hfu.repoId,
		})
	}
	ctx.BuildInfo.Modules[0].Artifacts = artifacts
	return SaveBuildInfo(ctx)
}

// GetArtifacts returns HuggingFace model/dataset files in JFrog Artifactory
func (hfu *HuggingFaceUpload) GetArtifacts(buildProperties string) ([]entities.Artifact, error) {
	serviceManager, err := utils.CreateServiceManager(hfu.serverDetails, -1, 0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create services manager: %w", err)
	}
	repoKey, err := GetRepoKeyFromHFEndpoint()
	if err != nil {
		return nil, err
	}
	repoTypePath := hfu.repoType + "s"
	revisionPattern := hfu.revision
	if !HasTimestamp(hfu.revision) {
		revisionPattern = hfu.revision + "_*"
	}
	aqlQuery := fmt.Sprintf(`items.find({"repo":"%s","path":{"$match":"%s/%s/%s/*"}}).include("repo","path","name","actual_sha1","actual_md5","sha256","type").sort({"$desc":["path"]})`,
		repoKey,
		repoTypePath,
		hfu.repoId,
		revisionPattern,
	)
	results, err := executeAqlQuery(serviceManager, aqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to search for HuggingFace artifacts: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	latestCreatedDir := results[0].Path
	var artifacts []entities.Artifact
	for _, resultItem := range results {
		artifacts = append(artifacts, entities.Artifact{
			Name: resultItem.Name,
			Type: resultItem.Type,
			Checksum: entities.Checksum{
				Sha1:   resultItem.Actual_Sha1,
				Md5:    resultItem.Actual_Md5,
				Sha256: resultItem.Sha256,
			},
		})
	}
	// Create content reader for the folder to set build properties
	reader, err := createContentReader(repoKey, latestCreatedDir, "", "folder")
	if err != nil {
		log.Warn("Failed to create content reader: ", err)
		return artifacts, nil
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Error(closeErr)
		}
	}()
	addBuildPropertiesOnArtifacts(serviceManager, reader, buildProperties)
	return artifacts, nil
}

// ServerDetails returns the server details configuration for the command
func (hfu *HuggingFaceUpload) ServerDetails() (*config.ServerDetails, error) {
	return hfu.serverDetails, nil
}

// CommandName returns the name of the command
func (hfu *HuggingFaceUpload) CommandName() string {
	return hfu.name
}

// NewHuggingFaceUpload creates a new instance of HFUploadCmd
func NewHuggingFaceUpload() *HuggingFaceUpload {
	return &HuggingFaceUpload{}
}

// SetFolderPath sets the folder path to upload for the upload command
func (hfu *HuggingFaceUpload) SetFolderPath(folderPath string) *HuggingFaceUpload {
	hfu.folderPath = folderPath
	return hfu
}

// SetRepoId sets the repository ID for the upload command
func (hfu *HuggingFaceUpload) SetRepoId(repoId string) *HuggingFaceUpload {
	hfu.repoId = repoId
	return hfu
}

// SetRevision sets the revision (branch, tag, or commit) for the upload command
func (hfu *HuggingFaceUpload) SetRevision(revision string) *HuggingFaceUpload {
	hfu.revision = revision
	return hfu
}

// SetServerDetails sets the revision (branch, tag, or commit) for the upload command
func (hfu *HuggingFaceUpload) SetServerDetails(serverDetails *config.ServerDetails) *HuggingFaceUpload {
	hfu.serverDetails = serverDetails
	return hfu
}

// SetBuildConfiguration sets the build configuration
func (hfu *HuggingFaceUpload) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *HuggingFaceUpload {
	hfu.buildConfiguration = buildConfiguration
	return hfu
}

// SetRepoType sets the repository type (model, dataset, or space) for the upload command
func (hfu *HuggingFaceUpload) SetRepoType(repoType string) *HuggingFaceUpload {
	hfu.repoType = repoType
	return hfu
}
