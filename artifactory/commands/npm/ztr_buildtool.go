package npm

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/zerotouchremediation"
)

const toolName = "npm"

type BuildTool struct {
	opts discoveryOptions
}

func NewBuildTool() BuildTool {
	return BuildTool{}
}

func (BuildTool) ToolName() string { return toolName }

func (BuildTool) RelevantCommands() []string {
	return []string{"install", "ci"}
}

func (t BuildTool) ProjectRoot(workingDir string) (string, error) {
	return discoverProjectRootWithOptions(workingDir, t.opts)
}

func (t BuildTool) EnsureLockfiles(ctx context.Context, projectRoot, command string, runner zerotouchremediation.CommandRunner, bootstrapArgs ...string) ([]string, error) {
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
		return nil, errorutils.CheckErrorf("Zero Touch Remediation requires %s or %s for npm ci (generate with npm install first)", lockfileName, shrinkwrapFileName)
	case "install":
		if runner == nil {
			return nil, errorutils.CheckErrorf("npm runner required to bootstrap %s", lockfileName)
		}
		log.Info("Zero Touch Remediation: generating ", lockfileName, " (lockfile was missing)")
		args := append([]string{"install", "--package-lock-only"}, bootstrapArgs...)
		if err := runner(ctx, projectRoot, args...); err != nil {
			return nil, errorutils.CheckError(err)
		}
		return []string{lockfileName}, nil
	default:
		return nil, nil
	}
}

func (BuildTool) DiscoverLockfiles(projectRoot string) ([]zerotouchremediation.Lockfile, error) {
	name, err := lockfileNameInDir(projectRoot)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(projectRoot, name))
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return []zerotouchremediation.Lockfile{{Path: name, Content: data}}, nil
}
