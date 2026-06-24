package maven

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

const toolName = "maven"

type BuildTool struct {
	goals []string
}

func NewBuildTool() BuildTool { return BuildTool{} }

func NewBuildToolWithGoals(goals []string) BuildTool {
	return BuildTool{goals: goals}
}

func (t BuildTool) discoveryOptions() discoveryOptions {
	pomFile, projectList := parseMavenCLIArgs(t.goals)
	return discoveryOptions{pomFile: pomFile, projectList: projectList}
}

func (BuildTool) ToolName() string { return toolName }

func (BuildTool) RelevantCommands() []string { return []string{"resolve"} }

func (t BuildTool) ProjectRoot(workingDir string) (string, error) {
	return discoverProjectRootWithOptions(workingDir, t.discoveryOptions())
}

func (t BuildTool) EnsureLockfiles(_ context.Context, projectRoot, _ string, _ healcomponents.CommandRunner, _ ...string) ([]string, error) {
	paths, err := discoverPomPathsWithOptions(projectRoot, t.discoveryOptions())
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errorutils.CheckErrorf("component resolution requires %s under %s", pomFileName, projectRoot)
	}
	return nil, nil
}

func (t BuildTool) DiscoverLockfiles(workingDir string) ([]healcomponents.Lockfile, error) {
	root, err := discoverProjectRootWithOptions(workingDir, t.discoveryOptions())
	if err != nil {
		return nil, err
	}
	relPaths, err := discoverPomPathsWithOptions(root, t.discoveryOptions())
	if err != nil {
		return nil, err
	}
	if len(relPaths) == 0 {
		return nil, errorutils.CheckErrorf("expected %s under %s", pomFileName, root)
	}
	var lockfiles []healcomponents.Lockfile
	for _, rel := range relPaths {
		data, readErr := os.ReadFile(filepath.Join(root, rel))
		if readErr != nil {
			return nil, errorutils.CheckError(readErr)
		}
		lockfiles = append(lockfiles, healcomponents.Lockfile{Path: rel, Content: data})
	}
	return lockfiles, nil
}
