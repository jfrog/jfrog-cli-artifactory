package cli

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"os/exec"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// HuggingFaceUpload represents a command to upload models or datasets to HuggingFace Hub
type HuggingFaceUpload struct {
	name          string
	folderPath    string
	repoId        string
	revision      string
	repoType      string
	serverDetails *config.ServerDetails
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
	cmd := exec.Command(pythonPath, "-c", pythonCmd)
	cmd.Dir = scriptDir
	output, err := cmd.CombinedOutput()
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
	return nil
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

// SetRepoType sets the repository type (model, dataset, or space) for the upload command
func (hfu *HuggingFaceUpload) SetRepoType(repoType string) *HuggingFaceUpload {
	hfu.repoType = repoType
	return hfu
}
