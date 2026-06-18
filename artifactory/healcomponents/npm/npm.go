package npm

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
)

const toolName = "npm"

type BuildTool struct {
	opts discoveryOptions
}

func NewBuildTool() BuildTool {
	return BuildTool{}
}

func NewBuildToolWithArgs(npmArgs []string) BuildTool {
	parsed := parseNpmCLIArgs(npmArgs)
	return BuildTool{opts: discoveryOptions{prefixDir: parsed.prefixDir}}
}

func NewBuildToolForPublish(workingDir, publishPath string, npmArgs []string) BuildTool {
	parsed := parseNpmCLIArgs(npmArgs)
	return BuildTool{opts: discoveryOptions{
		prefixDir:   parsed.prefixDir,
		publishPath: publishPath,
	}}
}

func (BuildTool) ToolName() string { return toolName }

func (BuildTool) RelevantCommands() []string {
	return []string{"install", "ci", "publish"}
}

func (t BuildTool) ProjectRoot(workingDir string) (string, error) {
	return discoverProjectRootWithOptions(workingDir, t.opts)
}

func (t BuildTool) EnsureLockfiles(ctx context.Context, projectRoot, command string, runner healcomponents.CommandRunner, bootstrapArgs ...string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(projectRoot, shrinkwrapFileName)); err == nil {
		return nil, nil
	} else if !os.IsNotExist(err) {
		return nil, errorutils.CheckError(err)
	}
	lockPath := filepath.Join(projectRoot, lockfileName)
	if _, err := os.Stat(lockPath); err == nil {
		return nil, nil
	} else if !os.IsNotExist(err) {
		return nil, errorutils.CheckError(err)
	}
	switch command {
	case "ci":
		return nil, errorutils.CheckErrorf("component resolution requires %s or %s for npm ci (generate with npm install first)", lockfileName, shrinkwrapFileName)
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

func (t BuildTool) DiscoverLockfiles(workingDir string) ([]healcomponents.Lockfile, error) {
	root, err := discoverProjectRootWithOptions(workingDir, t.opts)
	if err != nil {
		return nil, err
	}
	name, err := lockfileNameInDir(root)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return []healcomponents.Lockfile{{Path: name, Content: data}}, nil
}
