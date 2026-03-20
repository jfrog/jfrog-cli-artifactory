package cli

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type HuggingFaceRepositoryDetails struct {
	Key                   string   `json:"key"`
	Rclass                string   `json:"rclass"`
	PackageType           string   `json:"packageType"`
	DefaultDeploymentRepo string   `json:"defaultDeploymentRepo"`
	Repositories          []string `json:"repositories"`
}

//go:embed huggingface_download.py
var huggingfaceDownloadScript []byte

//go:embed huggingface_upload.py
var huggingfaceUploadScript []byte

// timestampPattern matches ISO 8601 timestamp format: _YYYY-MM-DDTHH:MM:SS.sssZ
var timestampPattern = regexp.MustCompile(`_\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

const (
	minPythonVersion = 3
	huggingfaceml    = "huggingfaceml"
	remote           = "remote"
	virtual          = "virtual"
	upload           = "upload"
)

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

// extractPythonScripts writes the embedded Python scripts to a temporary directory
// and returns its path. The caller is responsible for removing the directory when done.
func extractPythonScripts() (string, error) {
	tmpDir, err := os.MkdirTemp("", "jf-hf-*")
	if err != nil {
		return "", errorutils.CheckErrorf("failed to create temp directory for Python scripts: %w", err)
	}
	scripts := map[string][]byte{
		"huggingface_download.py": huggingfaceDownloadScript,
		"huggingface_upload.py":   huggingfaceUploadScript,
	}
	for name, data := range scripts {
		if err := os.WriteFile(filepath.Join(tmpDir, name), data, 0600); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", errorutils.CheckErrorf("failed to write embedded script %s: %w", name, err)
		}
	}
	return tmpDir, nil
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
	log.Debug("Python version", majorVersion, "verified (minimum required:", minPythonVersion, ")")
	return nil
}

// HasTimestamp checks if a revision ID already contains a timestamp suffix
// Example: "main_2026-02-09T09:01:17.646Z" returns true, "main" returns false
func HasTimestamp(revision string) bool {
	return timestampPattern.MatchString(revision)
}

func handleRepositoryResolution(serviceManager artifactory.ArtifactoryServicesManager, serverDetails *config.ServerDetails, command string) (string, error) {
	repoKey, err := GetRepoKeyFromHFEndpoint()
	if err != nil {
		return repoKey, err
	}
	repoDetails := HuggingFaceRepositoryDetails{}
	err = serviceManager.GetRepository(repoKey, &repoDetails)
	if err != nil {
		log.Error("Either repository doesn't exist or bad request")
		return "", err
	}
	if repoDetails.PackageType != huggingfaceml {
		return repoKey, errorutils.CheckErrorf("Given repository %s is not a huggingface's type repository", repoKey)
	}
	if repoDetails.Rclass == virtual {
		if repoDetails.DefaultDeploymentRepo == "" {
			return repoKey, errorutils.CheckErrorf("No default deployment repo specified for virtual repo: %s", repoKey)
		}
		hfEndpoint := serverDetails.GetArtifactoryUrl() + huggingfaceAPI + "/" + repoDetails.DefaultDeploymentRepo
		err = os.Setenv(HF_ENDPOINT, hfEndpoint)
		if err != nil {
			return repoKey, err
		}
		return repoDetails.DefaultDeploymentRepo, nil
	}
	if repoDetails.Rclass == remote && command == upload {
		return repoKey, errorutils.CheckError(errors.New("upload cannot be performed on remote repository"))
	}
	return repoKey, nil
}

// GetRepoKeyFromHFEndpoint extracts the repository key from HF_ENDPOINT environment variable
func GetRepoKeyFromHFEndpoint() (string, error) {
	endpoint := os.Getenv(HF_ENDPOINT)
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

type aqlResult struct {
	Results []aqlItem `json:"results"`
}

type aqlItem struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// createContentReader creates a ContentReader from the provided parameters
func createContentReader(repo, path, name, itemType string) (*content.ContentReader, error) {
	tempFile, err := os.CreateTemp("", "aql-results-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	filePath := tempFile.Name()
	err = json.NewEncoder(tempFile).Encode(aqlResult{
		Results: []aqlItem{{Repo: repo, Path: path, Name: name, Type: itemType}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}
	return content.NewContentReader(filePath, content.DefaultKey), nil
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
	log.Info("Collecting build info for executed huggingface", commandName, "command")
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
	log.Info("Build info saved locally.")
	return nil
}

// FindLatestRevision queries Artifactory for the most recent timestamped revision folder
// If revision already contains a timestamp it is returned as-is.
func FindLatestRevision(serviceManager artifactory.ArtifactoryServicesManager, repoKey, repoTypePath, repoId, revision string) (string, error) {
	if HasTimestamp(revision) {
		return revision, nil
	}
	namePattern := revision + "_*"
	aqlQuery := fmt.Sprintf(
		`items.find({"repo":"%s","type":"folder","path":"%s/%s","name":{"$match":"%s"}}).include("name").sort({"$desc":["name"]}).limit(1)`,
		repoKey, repoTypePath, repoId, namePattern,
	)
	results, err := utils.ExecuteAqlQuery(serviceManager, aqlQuery)
	if err != nil {
		return "", fmt.Errorf("failed to find latest revision folder: %w", err)
	}
	if len(results) == 0 {
		return "", nil
	}
	return results[0].Name, nil
}

func removeDuplicateDependencies(module *entities.Module) {
	dependenciesMap := make(map[string]entities.Dependency)
	var dependencies []entities.Dependency
	for _, dependency := range module.Dependencies {
		sha256 := dependency.Sha256
		if sha256 == "" {
			log.Debug("Missing Sha256 for dependency: ", dependency.Id, "so, skipping it from adding into build info.")
		}
		_, exist := dependenciesMap[sha256]
		if sha256 != "" && !exist {
			dependenciesMap[sha256] = dependency
			dependencies = append(dependencies, dependency)
		}
	}
	module.Dependencies = dependencies
}

func removeDuplicateArtifacts(module *entities.Module) {
	artifactsMap := make(map[string]entities.Artifact)
	var artifacts []entities.Artifact
	for _, artifact := range module.Artifacts {
		sha256 := artifact.Sha256
		if sha256 == "" {
			log.Debug("Missing Sha256 for artifact: ", artifact.Name, "so, skipping it from adding into build info.")
		}
		_, exist := artifactsMap[sha256]
		if sha256 != "" && !exist {
			artifactsMap[sha256] = artifact
			artifacts = append(artifacts, artifact)
		}
	}
	module.Artifacts = artifacts
}
