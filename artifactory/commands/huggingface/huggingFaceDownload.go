package cli

import (
	"encoding/json"
	"os/exec"

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
	name          string
	repoId        string
	revision      string
	repoType      string
	etagTimeout   int
	serverDetails *config.ServerDetails
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
		"repo_id":      hfd.repoId,
		"revision":     hfd.revision,
		"etag_timeout": hfd.etagTimeout,
	}
	if hfd.repoType != "" {
		args["repo_type"] = hfd.repoType
	} else {
		args["repo_type"] = "model"
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return errorutils.CheckErrorf("failed to marshal arguments to JSON: %w", err)
	}
	pythonCmd := BuildPythonDownloadCmd(string(argsJSON))
	log.Debug("Executing Python function to download ", args["repo_type"], ": ", hfd.repoId)
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
	log.Info("Downloaded successfully to:", result.ModelPath)
	return nil
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
