package python

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/build-info-go/utils/pythonutils"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python/dependencies"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/spf13/viper"
	"golang.org/x/exp/slices"
)

const (
	poetryConfigAuthPrefix = "http-basic."
	poetryConfigRepoPrefix = "repositories."
	pyproject              = "pyproject.toml"
)

type PoetryCommand struct {
	PythonCommand
}

func NewPoetryCommand() *PoetryCommand {
	return &PoetryCommand{
		PythonCommand: *NewPythonCommand(pythonutils.Poetry),
	}
}

func (pc *PoetryCommand) Run() (err error) {
	log.Info(fmt.Sprintf("Running Poetry %s.", pc.commandName))
	var buildConfiguration *buildUtils.BuildConfiguration
	pc.args, buildConfiguration, err = buildUtils.ExtractBuildDetailsFromArgs(pc.args)
	if err != nil {
		return err
	}
	pythonBuildInfo, err := buildUtils.PrepareBuildPrerequisites(buildConfiguration)
	if err != nil {
		return
	}
	defer func() {
		if pythonBuildInfo != nil && err != nil {
			err = errors.Join(err, pythonBuildInfo.Clean())
		}
	}()
	err = pc.SetPypiRepoUrlWithCredentials()
	if err != nil {
		return err
	}
	if pythonBuildInfo != nil {
		switch pc.commandName {
		case "install":
			return pc.install(buildConfiguration, pythonBuildInfo)
		case "publish":
			return pc.publish(buildConfiguration, pythonBuildInfo)
		default:
			// poetry native command
			return gofrogcmd.RunCmd(pc)

		}
	}
	return gofrogcmd.RunCmd(pc)
}

func (pc *PoetryCommand) install(buildConfiguration *buildUtils.BuildConfiguration, pythonBuildInfo *build.Build) (err error) {
	var pythonModule *build.PythonModule
	pythonModule, err = pythonBuildInfo.AddPythonModule("", pc.pythonTool)
	if err != nil {
		return
	}
	if buildConfiguration.GetModule() != "" {
		pythonModule.SetName(buildConfiguration.GetModule())
	}
	var localDependenciesPath string
	localDependenciesPath, err = config.GetJfrogDependenciesPath()
	if err != nil {
		return
	}
	pythonModule.SetLocalDependenciesPath(localDependenciesPath)
	pythonModule.SetUpdateDepsChecksumInfoFunc(pc.UpdateDepsChecksumInfoFunc)

	return errorutils.CheckError(pythonModule.RunInstallAndCollectDependencies(pc.args))
}

func (pc *PoetryCommand) publish(buildConfiguration *buildUtils.BuildConfiguration, pythonBuildInfo *build.Build) error {
	publishCmdArgs := append(slices.Clone(pc.args), "-r "+pc.repository)

	// Get build name and number (already extracted from CLI arguments)
	// Since buildConfiguration is created from CLI args, these should be available directly
	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	// Get current working directory
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}

	// Use FlexPack to collect dependencies with checksums
	if buildName != "" && buildNumber != "" {
		log.Info("Using native approach to collect Poetry dependencies...")

		// Create FlexPack Poetry configuration
		config := flexpack.PoetryConfig{
			WorkingDirectory: workingDir,
		}

		// Create Poetry FlexPack instance
		poetryFlex, err := flexpack.NewPoetryFlexPack(config)
		if err != nil {
			return fmt.Errorf("failed to create Poetry FlexPack: %w", err)
		}

		// Collect build info using FlexPack
		flexBuildInfo, err := poetryFlex.CollectBuildInfo(buildName, buildNumber)
		if err != nil {
			return fmt.Errorf("failed to collect build info with FlexPack: %w", err)
		}

		// Save FlexPack build info to be picked up by rt bp
		return pc.saveFlexPackBuildInfo(flexBuildInfo)
	}

	// Run the publish command to upload artifacts
	pc.args = publishCmdArgs
	err = gofrogcmd.RunCmd(pc)
	if err != nil {
		return err
	}

	// After successful publish, collect artifacts information
	if buildName != "" && buildNumber != "" {
		return pc.collectPublishedArtifacts(buildConfiguration, pythonBuildInfo, workingDir)
	}

	return nil
}

// saveFlexPackBuildInfo saves FlexPack build info for jfrog-cli rt bp compatibility
func (pc *PoetryCommand) saveFlexPackBuildInfo(buildInfo *entities.BuildInfo) error {
	// Create build-info service
	service := build.NewBuildInfoService()

	// Create or get build
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, "")
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}

	// Save the complete build info (this will be loaded by rt bp)
	return buildInstance.SaveBuildInfo(buildInfo)
}

// collectPublishedArtifacts collects information about artifacts that were published
func (pc *PoetryCommand) collectPublishedArtifacts(buildConfiguration *buildUtils.BuildConfiguration, pythonBuildInfo *build.Build, workingDir string) error {
	// Get the build directory from pyproject.toml configuration or use default
	buildDir, err := pc.getBuildDirectoryFromPyproject(workingDir)
	if err != nil {
		log.Debug("Failed to read build directory from pyproject.toml, using default 'dist': " + err.Error())
		buildDir = filepath.Join(workingDir, "dist")
	}

	// Look for built artifacts in the build directory
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		log.Debug(fmt.Sprintf("No build directory found at %s, skipping artifact collection", buildDir))
		return nil
	}

	// Get module name
	moduleName := buildConfiguration.GetModule()
	if moduleName == "" {
		// Try to determine module name from pyproject.toml
		if name, err := pc.getProjectNameFromPyproject(workingDir); err == nil {
			moduleName = name
		} else {
			moduleName = "poetry-module"
		}
	}

	// Find artifacts in build directory
	artifacts, err := pc.findDistArtifacts(buildDir)
	if err != nil {
		return err
	}

	if len(artifacts) > 0 {
		log.Debug(fmt.Sprintf("Found %d artifacts to add to build info", len(artifacts)))
		// Add artifacts to build info
		return pythonBuildInfo.AddArtifacts(moduleName, "pypi", artifacts...)
	}

	return nil
}

// getBuildDirectoryFromPyproject reads the build directory configuration from pyproject.toml
func (pc *PoetryCommand) getBuildDirectoryFromPyproject(workingDir string) (string, error) {
	pyprojectPath := filepath.Join(workingDir, pyproject)
	viper.SetConfigType("toml")
	viper.SetConfigFile(pyprojectPath)
	if err := viper.ReadInConfig(); err != nil {
		return "", err
	}

	// Check for build directory configuration in [tool.poetry.build] section
	buildDir := viper.GetString("tool.poetry.build.directory")
	if buildDir != "" {
		// If it's a relative path, make it absolute relative to working directory
		if !filepath.IsAbs(buildDir) {
			buildDir = filepath.Join(workingDir, buildDir)
		}
		return buildDir, nil
	}

	// Default to "dist" directory if not configured
	return filepath.Join(workingDir, "dist"), nil
}

// getProjectNameFromPyproject extracts project name from pyproject.toml
func (pc *PoetryCommand) getProjectNameFromPyproject(workingDir string) (string, error) {
	pyprojectPath := filepath.Join(workingDir, pyproject)
	viper.SetConfigType("toml")
	viper.SetConfigFile(pyprojectPath)
	if err := viper.ReadInConfig(); err != nil {
		return "", err
	}

	name := viper.GetString("tool.poetry.name")
	if name == "" {
		return "", fmt.Errorf("no project name found in pyproject.toml")
	}

	return name, nil
}

// findDistArtifacts finds and creates artifact entries for files in dist directory
func (pc *PoetryCommand) findDistArtifacts(distDir string) ([]entities.Artifact, error) {
	var artifacts []entities.Artifact

	entries, err := os.ReadDir(distDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// Only include wheel and tar.gz files
		artifactType := getArtifactType(filename)
		if artifactType == "unknown" {
			continue
		}

		filePath := filepath.Join(distDir, filename)

		// Create artifact entry
		artifact := entities.Artifact{
			Name: filename,
			Path: filePath,
			Type: artifactType,
		}

		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

// getArtifactType determines the artifact type based on file extension
func getArtifactType(filename string) string {
	if strings.HasSuffix(filename, ".whl") {
		return "wheel"
	} else if strings.HasSuffix(filename, ".tar.gz") {
		return "sdist"
	}
	return "unknown"
}

func (pc *PoetryCommand) UpdateDepsChecksumInfoFunc(dependenciesMap map[string]entities.Dependency, srcPath string) error {
	servicesManager, err := utils.CreateServiceManager(pc.serverDetails, -1, 0, false)
	if err != nil {
		return err
	}
	return dependencies.UpdateDepsChecksumInfo(dependenciesMap, srcPath, servicesManager, pc.repository)
}

func (pc *PoetryCommand) SetRepo(repo string) *PoetryCommand {
	pc.repository = repo
	return pc
}

func (pc *PoetryCommand) SetArgs(arguments []string) *PoetryCommand {
	pc.args = arguments
	return pc
}

func (pc *PoetryCommand) SetCommandName(commandName string) *PoetryCommand {
	pc.commandName = commandName
	return pc
}

func (pc *PoetryCommand) SetPypiRepoUrlWithCredentials() error {
	rtUrl, username, password, err := GetPypiRepoUrlWithCredentials(pc.serverDetails, pc.repository, false)
	if err != nil {
		return err
	}
	if password != "" {
		return ConfigPoetryRepo(
			rtUrl.Scheme+"://"+rtUrl.Host+rtUrl.Path,
			username,
			password,
			pc.repository)
	}
	return nil
}

func ConfigPoetryRepo(url, username, password, configRepoName string) error {
	err := RunPoetryConfig(url, username, password, configRepoName)
	if err != nil {
		return err
	}

	// Add the repository config to the pyproject.toml
	currentDir, err := os.Getwd()
	if err != nil {
		return errorutils.CheckError(err)
	}
	if err = addRepoToPyprojectFile(filepath.Join(currentDir, pyproject), configRepoName, url); err != nil {
		return err
	}
	return poetryUpdate()
}

func RunPoetryConfig(url, username, password, configRepoName string) error {
	// Add the poetry repository config
	// poetry config repositories.<repo-name> https://<your-artifactory-url>/artifactory/api/pypi/<repo-name>/simple
	err := RunConfigCommand(project.Poetry, []string{poetryConfigRepoPrefix + configRepoName, url})
	if err != nil {
		return err
	}

	// Set the poetry repository credentials
	// poetry config http-basic.<repo-name> <user> <password/token>
	return RunConfigCommand(project.Poetry, []string{poetryConfigAuthPrefix + configRepoName, username, password})
}

func poetryUpdate() (err error) {
	log.Info("Running Poetry update")
	cmd := gofrogcmd.NewCommand("poetry", "update", []string{})
	err = gofrogcmd.RunCmd(cmd)
	if err != nil {
		return errorutils.CheckErrorf("Poetry config command failed with: %s", err.Error())
	}
	return
}

func addRepoToPyprojectFile(filepath, poetryRepoName, repoUrl string) error {
	viper.SetConfigType("toml")
	viper.SetConfigFile(filepath)
	if err := viper.ReadInConfig(); err != nil {
		return errorutils.CheckErrorf("Failed to read pyproject.toml: %s", err.Error())
	}
	viper.Set("tool.poetry.source", []map[string]string{{"name": poetryRepoName, "url": repoUrl}})
	if err := viper.WriteConfig(); err != nil {
		return errorutils.CheckErrorf("Failed to add tool.poetry.source to pyproject.toml: %s", err.Error())

	}
	log.Info(fmt.Sprintf("Added tool.poetry.source name:%q url:%q", poetryRepoName, repoUrl))
	return nil
}

func (pc *PoetryCommand) CommandName() string {
	return "rt_python_poetry"
}

func (pc *PoetryCommand) SetServerDetails(serverDetails *config.ServerDetails) *PoetryCommand {
	pc.serverDetails = serverDetails
	return pc
}

func (pc *PoetryCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

func (pc *PoetryCommand) GetCmd() *exec.Cmd {
	var cmd []string
	cmd = append(cmd, string(pc.pythonTool))
	cmd = append(cmd, pc.commandName)
	cmd = append(cmd, pc.args...)
	return exec.Command(cmd[0], cmd[1:]...)
}

func (pc *PoetryCommand) GetEnv() map[string]string {
	return map[string]string{}
}

func (pc *PoetryCommand) GetStdWriter() io.WriteCloser {
	return nil
}

func (pc *PoetryCommand) GetErrWriter() io.WriteCloser {
	return nil
}
