package cli

import (
	"encoding/json"
	"fmt"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// timestampPattern matches ISO 8601 timestamp format: _YYYY-MM-DDTHH:MM:SS.sssZ
var timestampPattern = regexp.MustCompile(`_\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

const minPythonVersion = 3

// PythonScriptTemplate is the base template for executing Python functions via importlib.
// It accepts format arguments: module name, function name, JSON args, and success output expression.
const PythonScriptTemplate = `import sys,json,importlib
try:
	m=importlib.import_module("%s")
	f=getattr(m,"%s")
	%s
except Exception as e:
	print(json.dumps({"success":False,"error":str(e)}))
	sys.exit(1)`

// PythonUploadSuccessBlock is the success block for upload operations
// Using raw string (r"...") to prevent Python from interpreting backslashes in Windows paths as escape sequences
const PythonUploadSuccessBlock = `f(**json.loads(r"""%s"""))
	print(json.dumps({"success":True}))`

// PythonDownloadSuccessBlock is the success block for download operations
// Using raw string (r"...") to prevent Python from interpreting backslashes in Windows paths as escape sequences
const PythonDownloadSuccessBlock = `r=f(**json.loads(r"""%s"""))
	print(json.dumps({"success":True,"model_path":r}))`

// BuildPythonUploadCmd builds the Python command string for upload operations
func BuildPythonUploadCmd(argsJSON string) string {
	successBlock := fmt.Sprintf(PythonUploadSuccessBlock, argsJSON)
	return fmt.Sprintf(PythonScriptTemplate, "huggingface_upload", "upload", successBlock)
}

// BuildPythonDownloadCmd builds the Python command string for download operations
func BuildPythonDownloadCmd(argsJSON string) string {
	successBlock := fmt.Sprintf(PythonDownloadSuccessBlock, argsJSON)
	return fmt.Sprintf(PythonScriptTemplate, "huggingface_download", "download", successBlock)
}

// getHuggingFaceScriptPath returns the absolute path to the directory containing Python scripts
func getHuggingFaceScriptPath(scriptName string) (string, error) {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		return "", errorutils.CheckErrorf("failed to get current file path")
	}
	scriptDir := filepath.Dir(filename)
	scriptPath := filepath.Join(scriptDir, scriptName)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", errorutils.CheckErrorf("Python script not found: %s in directory: %s", scriptName, scriptDir)
	}
	return scriptDir, nil
}

// GetPythonPath finds a valid Python 3+ interpreter in PATH.
// It first tries "python3", then falls back to "python", and verifies the version is 3+.
func GetPythonPath() (string, error) {
	pythonPath, err := exec.LookPath("python3")
	if err == nil {
		log.Debug("Found Python interpreter: ", pythonPath)
		if err := verifyPythonVersion(pythonPath); err == nil {
			return pythonPath, nil
		}
	}
	pythonPath, err = exec.LookPath("python")
	if err != nil {
		return "", errorutils.CheckErrorf("neither python3 nor python found in PATH. Please ensure Python 3 is installed and available in your PATH")
	}
	log.Debug("Found Python interpreter: ", pythonPath)
	if err := verifyPythonVersion(pythonPath); err != nil {
		return "", err
	}
	return pythonPath, nil
}

// verifyPythonVersion checks that the Python interpreter is version 3 or higher
func verifyPythonVersion(pythonPath string) error {
	cmd := exec.Command(pythonPath, "-c", "import sys; print(sys.version_info.major)")
	output, err := cmd.Output()
	if err != nil {
		return errorutils.CheckErrorf("failed to get Python version: %w", err)
	}
	versionStr := strings.TrimSpace(string(output))
	majorVersion, err := strconv.Atoi(versionStr)
	if err != nil {
		return errorutils.CheckErrorf("failed to parse Python version '%s': %w", versionStr, err)
	}
	if majorVersion < minPythonVersion {
		return errorutils.CheckErrorf("Python version %d found, but version %d or higher is required", majorVersion, minPythonVersion)
	}
	log.Debug("Python version ", majorVersion, " verified (minimum required: ", minPythonVersion, ")")
	return nil
}

// HasTimestamp checks if a revision ID already contains a timestamp suffix
// Example: "main_2026-02-09T09:01:17.646Z" returns true, "main" returns false
func HasTimestamp(revision string) bool {
	return timestampPattern.MatchString(revision)
}

// GetRepoKeyFromHFEndpoint extracts the repository key from HF_ENDPOINT environment variable
func GetRepoKeyFromHFEndpoint() (string, error) {
	endpoint := os.Getenv("HF_ENDPOINT")
	if endpoint == "" {
		return "", errorutils.CheckErrorf("HF_ENDPOINT environment variable is not set")
	}
	endpoint = strings.TrimSuffix(endpoint, "/")
	parts := strings.Split(endpoint, "/")
	if len(parts) == 0 {
		return "", errorutils.CheckErrorf("invalid HF_ENDPOINT format: %s", endpoint)
	}
	repoKey := parts[len(parts)-1]
	if repoKey == "" {
		return "", errorutils.CheckErrorf("could not extract repo key from HF_ENDPOINT: %s", endpoint)
	}
	log.Debug("Extracted repo key from HF_ENDPOINT: ", repoKey)
	return repoKey, nil
}

// updateReaderContents updates the reader contents by writing the specified JSON value to all file paths
func updateReaderContents(reader *content.ContentReader, repo, path, name string) error {
	if reader == nil {
		return fmt.Errorf("reader is nil")
	}
	jsonData := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"repo": repo,
				"path": path,
				"name": name,
				"type": "folder",
			},
		},
	}
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	filesPaths := reader.GetFilesPaths()
	for _, filePath := range filesPaths {
		err := os.WriteFile(filePath, jsonBytes, 0644)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to write JSON to file %s: %s", filePath, err))
			continue
		}
		log.Debug(fmt.Sprintf("Successfully updated file %s with JSON content", filePath))
	}
	return nil
}

// executeAqlQuery executes an AQL query and parses the JSON response
func executeAqlQuery(serviceManager artifactory.ArtifactoryServicesManager, aqlQuery string) ([]servicesUtils.ResultItem, error) {
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

func addBuildPropertiesOnArtifacts(serviceManager artifactory.ArtifactoryServicesManager, reader *content.ContentReader, buildProps string) {
	propsParams := services.PropsParams{
		Reader:      reader,
		Props:       buildProps,
		IsRecursive: true,
	}
	_, _ = serviceManager.SetProps(propsParams)
}

// BuildInfoContext holds the common build info components needed for both upload and download
type BuildInfoContext struct {
	BuildName   string
	BuildNumber string
	Project     string
	BuildInfo   *entities.BuildInfo
}

// GetBuildInfoContext extracts common build configuration and creates build info context
// Returns nil if build info collection is not enabled
func GetBuildInfoContext(buildConfig *buildUtils.BuildConfiguration, commandName string) (*BuildInfoContext, error) {
	if buildConfig == nil {
		return nil, nil
	}
	isCollectBuildInfo, err := buildConfig.IsCollectBuildInfo()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	if !isCollectBuildInfo {
		return nil, nil
	}
	log.Info("Collecting build info for executed huggingface ", commandName, " command")
	buildName, err := buildConfig.GetBuildName()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	buildNumber, err := buildConfig.GetBuildNumber()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	project := buildConfig.GetProject()
	buildInfoService := buildUtils.CreateBuildInfoService()
	build, err := buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create build info: %w", err)
	}
	buildInfo, err := build.ToBuildInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get build info: %w", err)
	}
	return &BuildInfoContext{
		BuildName:   buildName,
		BuildNumber: buildNumber,
		Project:     project,
		BuildInfo:   buildInfo,
	}, nil
}

// SaveBuildInfo saves the build info from the context
func SaveBuildInfo(ctx *BuildInfoContext) error {
	if err := buildUtils.SaveBuildInfo(ctx.BuildName, ctx.BuildNumber, ctx.Project, ctx.BuildInfo); err != nil {
		log.Warn("Failed to save build info: ", err.Error())
		return err
	}
	log.Info("Build info saved locally. Use 'jf rt bp", ctx.BuildName, ctx.BuildNumber, "' to publish it to Artifactory.")
	return nil
}
