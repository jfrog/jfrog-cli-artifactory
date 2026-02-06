package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

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
const PythonUploadSuccessBlock = `f(**json.loads("""%s"""))
	print(json.dumps({"success":True}))`

// PythonDownloadSuccessBlock is the success block for download operations
const PythonDownloadSuccessBlock = `r=f(**json.loads("""%s"""))
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
