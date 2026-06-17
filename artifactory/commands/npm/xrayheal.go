package npm

import (
	"context"
	"encoding/json"
	"strings"
	"os"
	"fmt"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"

	gofrogcmd "github.com/jfrog/gofrog/io"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/xray"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
)

const (
	toolName = "npm"
	lockfileName = "package-lock.json"
)

func (ca *CommonArgs) runXrayComponentHealing(ctx context.Context, command, workingDir string, npmArgs []string) (restore func() error, healed bool, err error) {
	noop := func() error { return nil }
	if healcomponents.IsComponentResolutionDisabled() {
		return noop, false, nil
	}
	if command == "install" && isSinglePackageInstall(npmArgs) {
		return noop, false, nil
	}
	resolverRepo, err := ca.resolverRepoForResolution(command)
	if err != nil || resolverRepo == "" {
		log.Debug("Xray component healing skipped: could not determine resolver repo: " + err.Error())
		return noop, false, nil
	}
	var projectKey string
	if ca.buildConfiguration != nil {
		projectKey = ca.buildConfiguration.GetProject()
	}
	xrayManager, err := xray.CreateXrayServiceManager(ca.serverDetails, xray.WithScopedProjectKey(projectKey))
	if err != nil {
		log.Debug("Xray component healing skipped: could not create Xray service manager: " + err.Error())
		return noop, false, nil
	}
	return healcomponents.RunIfEnabled(ctx, xrayManager, resolverRepo, NewNpmBuildTool(), command, workingDir, ca.npmBootstrapRunner(), extractWorkspaceBootstrapArgs(npmArgs)...)
}

// resolverRepoForResolution returns the Artifactory virtual repo for dependency policy scope.
func (ca *CommonArgs) resolverRepoForResolution(command string) (string, error) {
	if command != "publish" && ca.repo != "" {
		return ca.repo, nil
	}
	if ca.configFilePath != "" {
		vConfig, err := project.ReadConfigFile(ca.configFilePath, project.YAML)
		if err != nil {
			return "", fmt.Errorf("failed to read config file: %w", err)
		}
		resolverConfig, err := project.GetRepoConfigByPrefix(ca.configFilePath, project.ProjectConfigResolverPrefix, vConfig)
		if err != nil {
			return "", fmt.Errorf("failed to get resolver config: %w", err)
		}
		return resolverConfig.TargetRepo(), nil
	}
	if ca.executablePath != "" {
		registryURL, err := ca.getNpmRegistryURL()
		if err != nil {
			return "", fmt.Errorf("failed to get registry URL: %w", err)
		}
		return extractRepoName(registryURL)
	}
	return ca.repo, nil
}

func (ca *CommonArgs) getNpmRegistryURL() (string, error) {
	configCommand := gofrogcmd.Command{
		Executable: ca.executablePath,
		CmdName:    "config",
		CmdArgs:    []string{"get", "registry"},
	}
	data, err := configCommand.RunWithOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (ca *CommonArgs) npmBootstrapRunner() healcomponents.CommandRunner {
	return func(ctx context.Context, projectRoot string, args ...string) error {
		return runNpmAt(ctx, ca.executablePath, projectRoot, args...)
	}
}

func runNpmAt(_ context.Context, executablePath, projectRoot string, args ...string) error {
	if len(args) == 0 {
		return nil
	}
	cmd := gofrogcmd.NewCommand(executablePath, args[0], args[1:])
	cmd.Dir = projectRoot
	_, err := cmd.RunWithOutput()
	return err
}

func extractWorkspaceBootstrapArgs(npmArgs []string) []string {
	var bootstrapArgs []string
	for i := 0; i < len(npmArgs); i++ {
		arg := npmArgs[i]
		if arg == "--workspaces" || arg == "-w" {
			bootstrapArgs = append(bootstrapArgs, arg)
			continue
		}
		if strings.HasPrefix(arg, "--workspace=") {
			bootstrapArgs = append(bootstrapArgs, arg)
			continue
		}
		if arg == "--workspace" && i+1 < len(npmArgs) {
			bootstrapArgs = append(bootstrapArgs, arg, npmArgs[i+1])
			i++
		}
	}
	return bootstrapArgs
}

func isSinglePackageInstall(npmArgs []string) bool {
	for _, arg := range npmArgs {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}


type NpmBuildTool struct{}

func NewNpmBuildTool() NpmBuildTool {
	return NpmBuildTool{}
}

func (NpmBuildTool) ToolName() string {
	return toolName
}

func (NpmBuildTool) RelevantCommands() []string {
	return []string{"install", "ci", "publish"}
}

func (NpmBuildTool) ProjectRoot(workingDir string) (string, error) {
	return discoverProjectRoot(workingDir)
}

func (t NpmBuildTool) EnsureLockfiles(ctx context.Context, projectRoot, command string, runner healcomponents.CommandRunner, bootstrapArgs ...string) ([]string, error) {
	lockPath := filepath.Join(projectRoot, lockfileName)
	if _, err := os.Stat(lockPath); err == nil {
		return nil, nil
	} else if !os.IsNotExist(err) {
		return nil, errorutils.CheckError(err)
	}
	// Lockfile is missing, we need to bootstrap it
	switch command {
	case "ci":
		return nil, errorutils.CheckErrorf("component resolution requires %s for npm ci (generate with npm install first)", lockfileName)
	case "install", "publish":
		if runner == nil {
			return nil, errorutils.CheckErrorf("npm runner required to bootstrap %s", lockfileName)
		}
		log.Info("Component resolution: generating ", lockfileName, " (lockfile was missing)")
		args := append([]string{"install", "--package-lock-only"}, bootstrapArgs...)
		if err := runner(ctx, projectRoot, args...); err != nil {
			return nil, errorutils.CheckError(err)
		}
		return []string{lockfileName}, nil
	default:
		return nil, nil
	}
}

func (NpmBuildTool) DiscoverLockfiles(workingDir string) ([]healcomponents.Lockfile, error) {
	root, err := discoverProjectRoot(workingDir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, lockfileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, errorutils.CheckErrorf("expected %s under %s after EnsureLockfiles", lockfileName, root)
	}
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return []healcomponents.Lockfile{{Path: lockfileName, Content: data}}, nil
}


// discoverProjectRoot walks up from workingDir to find the npm workspace or lockfile root.
func discoverProjectRoot(workingDir string) (string, error) {
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", errorutils.CheckError(err)
	}

	dir := abs
	var firstPackageJSON string
	for {
		pkgPath := filepath.Join(dir, "package.json")
		if _, statErr := os.Stat(pkgPath); statErr == nil {
			if firstPackageJSON == "" {
				firstPackageJSON = dir
			}
			data, readErr := os.ReadFile(pkgPath)
			if readErr != nil {
				return "", errorutils.CheckError(readErr)
			}
			var pkg struct {
				Workspaces any `json:"workspaces"`
			}
			if json.Unmarshal(data, &pkg) == nil && pkg.Workspaces != nil {
				return dir, nil
			}
		}

		lockPath := filepath.Join(dir, "package-lock.json")
		if _, statErr := os.Stat(lockPath); statErr == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if firstPackageJSON != "" {
		return firstPackageJSON, nil
	}
	return abs, nil
}
