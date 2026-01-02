package gradle

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	buildinfoflexpack "github.com/jfrog/build-info-go/flexpack/gradle"
	flexpackgradle "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/flexpack/gradle"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	artifactoryutils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/dependencies"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/spf13/viper"
)

//go:embed resources/jfrog.init.gradle
var gradleInitScript string

const (
	usePlugin  = "useplugin"
	useWrapper = "usewrapper"

	UserHomeEnv    = "GRADLE_USER_HOME"
	InitScriptName = "jfrog.init.gradle"
)

type GradleCommand struct {
	tasks              []string
	configPath         string
	configuration      *build.BuildConfiguration
	serverDetails      *config.ServerDetails
	threads            int
	detailedSummary    bool
	xrayScan           bool
	scanOutputFormat   format.OutputFormat
	result             *commandsutils.Result
	deploymentDisabled bool
	// File path for Gradle extractor in which all build's artifacts details will be listed at the end of the build.
	buildArtifactsDetailsFile string
}

func NewGradleCommand() *GradleCommand {
	return &GradleCommand{}
}

// Returns the ServerDetails. The information returns from the config file provided.
func (gc *GradleCommand) ServerDetails() (*config.ServerDetails, error) {
	// Get the serverDetails from the config file.
	var err error
	if gc.serverDetails == nil {
		vConfig, err := project.ReadConfigFile(gc.configPath, project.YAML)
		if err != nil {
			return nil, err
		}
		gc.serverDetails, err = build.GetServerDetails(vConfig)
		if err != nil {
			return nil, err
		}
	}
	return gc.serverDetails, err
}

func (gc *GradleCommand) SetServerDetails(serverDetails *config.ServerDetails) *GradleCommand {
	gc.serverDetails = serverDetails
	return gc
}

func (gc *GradleCommand) init() (vConfig *viper.Viper, err error) {
	// Read config
	vConfig, err = project.ReadConfigFile(gc.configPath, project.YAML)
	if err != nil {
		return
	}
	if gc.IsXrayScan() && !vConfig.IsSet("deployer") {
		err = errorutils.CheckErrorf("Conditional upload can only be performed if deployer is set in the config")
		return
	}
	// Gradle extractor is needed to run, in order to get the details of the build's artifacts.
	// Gradle's extractor deploy build artifacts. This should be disabled since there is no intent to deploy anything or deploy upon Xray scan results.
	gc.deploymentDisabled = gc.IsXrayScan() || !vConfig.IsSet("deployer")
	if gc.shouldCreateBuildArtifactsFile() {
		// Created a file that will contain all the details about the build's artifacts
		tempFile, err := fileutils.CreateTempFile()
		if err != nil {
			return nil, err
		}
		// If this is a Windows machine there is a need to modify the path for the build info file to match Java syntax with double \\
		gc.buildArtifactsDetailsFile = ioutils.DoubleWinPathSeparator(tempFile.Name())
		if err = tempFile.Close(); errorutils.CheckError(err) != nil {
			return nil, err
		}
	}
	return
}

// Gradle extractor generates the details of the build's artifacts.
// This is required for Xray scan and for the detailed summary.
// We can either scan or print the generated artifacts.
func (gc *GradleCommand) shouldCreateBuildArtifactsFile() bool {
	return (gc.IsDetailedSummary() && !gc.deploymentDisabled) || gc.IsXrayScan()
}

func (gc *GradleCommand) Run() error {
	if artifactoryutils.ShouldRunNative(gc.configPath) {
		return gc.runWithGradleNative()
	}

	vConfig, err := gc.init()
	if err != nil {
		return err
	}
	err = runGradle(vConfig, gc.tasks, gc.buildArtifactsDetailsFile, gc.configuration, gc.threads, gc.IsXrayScan())
	if err != nil {
		return err
	}
	if gc.buildArtifactsDetailsFile != "" {
		err = gc.unmarshalDeployableArtifacts(gc.buildArtifactsDetailsFile)
		if err != nil {
			return err
		}
		if gc.IsXrayScan() {
			return gc.conditionalUpload()
		}
	}
	return nil
}

func (gc *GradleCommand) unmarshalDeployableArtifacts(filesPath string) error {
	result, err := commandsutils.UnmarshalDeployableArtifacts(filesPath, gc.configPath, gc.IsXrayScan())
	if err != nil {
		return err
	}
	gc.setResult(result)
	return nil
}

// runWithGradleNative executes Gradle using FlexPack for dependency resolution and build info collection
func (gc *GradleCommand) runWithGradleNative() error {
	log.Debug("Gradle native implementation activated")

	// Get working directory - default to current directory
	workingDir, err := os.Getwd()
	if err != nil {
		return errorutils.CheckError(err)
	}

	// Check if a build file is specified via -b or --build-file flag
	flexpackWorkingDir := workingDir
	if buildFilePath := extractBuildFilePath(gc.tasks); buildFilePath != "" {
		buildFileDir := filepath.Dir(buildFilePath)
		if filepath.IsAbs(buildFileDir) {
			flexpackWorkingDir = buildFileDir
		} else {
			flexpackWorkingDir = filepath.Join(workingDir, buildFileDir)
		}
		log.Debug(fmt.Sprintf("Using build file directory as FlexPack working directory: %s", flexpackWorkingDir))
	}

	gradleExecPath, err := buildinfoflexpack.GetGradleExecutablePath(workingDir)
	if err != nil {
		return fmt.Errorf("failed to find Gradle executable: %w", err)
	}

	cmd := exec.Command(gradleExecPath, gc.tasks...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Run(); err != nil {
		log.Error("Failed to execute Gradle command: " + err.Error())
		return errorutils.CheckError(err)
	}

	if gc.configuration != nil {
		isCollect, err := gc.configuration.IsCollectBuildInfo()
		if err != nil {
			return err
		}
		if isCollect {
			log.Debug("Collecting build info for executed command...")
			buildName, buildNumber, err := gc.getBuildNameAndNumber()
			if err != nil {
				return err
			}

			// Call FlexPack collection using the flexpack working directory
			if err := flexpackgradle.CollectGradleBuildInfoWithFlexPack(flexpackWorkingDir, buildName, buildNumber, gc.tasks, gc.configuration, gc.serverDetails); err != nil {
				log.Warn("Failed to collect Gradle build info with Flexpack:")
			}
		}
	}
	return nil
}

// It looks for -b/--build-file flags (build file path) and -p/--project-dir flags (project directory).
func extractBuildFilePath(tasks []string) string {
	for i, task := range tasks {
		// Check for -b<path> (no space)
		if strings.HasPrefix(task, "-b") && len(task) > 2 && task[2] != '-' {
			return task[2:]
		}
		// Check for --build-file=<path>
		if strings.HasPrefix(task, "--build-file=") {
			return strings.TrimPrefix(task, "--build-file=")
		}
		// Check for -b <path> or --build-file <path> (with space)
		if (task == "-b" || task == "--build-file") && i+1 < len(tasks) {
			return tasks[i+1]
		}
		// Check for -p<path> (no space) - project directory
		if strings.HasPrefix(task, "-p") && len(task) > 2 && task[2] != '-' {
			// For -p, the path is already a directory, append a dummy file to get consistent behavior
			return filepath.Join(task[2:], "build.gradle")
		}
		// Check for --project-dir=<path>
		if strings.HasPrefix(task, "--project-dir=") {
			dir := strings.TrimPrefix(task, "--project-dir=")
			return filepath.Join(dir, "build.gradle")
		}
		// Check for -p <path> or --project-dir <path> (with space)
		if (task == "-p" || task == "--project-dir") && i+1 < len(tasks) {
			return filepath.Join(tasks[i+1], "build.gradle")
		}
	}
	return ""
}

// ConditionalUpload will scan the artifact using Xray and will upload them only if the scan passes with no
// violation.
func (gc *GradleCommand) conditionalUpload() error {
	// Initialize the server details (from config) if it hasn't been initialized yet.
	_, err := gc.ServerDetails()
	if err != nil {
		return err
	}
	binariesSpecFile, pomSpecFile, err := commandsutils.ScanDeployableArtifacts(gc.result, gc.serverDetails, gc.threads, gc.scanOutputFormat)
	// If the detailed summary wasn't requested, the reader should be closed here.
	// (otherwise it will be closed by the detailed summary print method)
	if !gc.detailedSummary {
		e := gc.result.Reader().Close()
		if e != nil {
			return e
		}
	} else {
		gc.result.Reader().Reset()
	}
	if err != nil {
		return err
	}
	// The case scan failed
	if binariesSpecFile == nil {
		return nil
	}
	// First upload binaries
	if len(binariesSpecFile.Files) > 0 {
		uploadCmd := generic.NewUploadCommand()
		uploadConfiguration := new(utils.UploadConfiguration)
		uploadConfiguration.Threads = gc.threads
		uploadCmd.SetUploadConfiguration(uploadConfiguration).SetBuildConfiguration(gc.configuration).SetSpec(binariesSpecFile).SetServerDetails(gc.serverDetails)
		err = uploadCmd.Run()
		if err != nil {
			return err
		}
	}
	if len(pomSpecFile.Files) > 0 {
		// Then Upload pom.xml's
		uploadCmd := generic.NewUploadCommand()
		uploadCmd.SetBuildConfiguration(gc.configuration).SetSpec(pomSpecFile).SetServerDetails(gc.serverDetails)
		err = uploadCmd.Run()
	}
	return err
}

func (gc *GradleCommand) CommandName() string {
	return "rt_gradle"
}

func (gc *GradleCommand) SetConfiguration(configuration *build.BuildConfiguration) *GradleCommand {
	gc.configuration = configuration
	return gc
}

func (gc *GradleCommand) SetConfigPath(configPath string) *GradleCommand {
	gc.configPath = configPath
	return gc
}

func (gc *GradleCommand) SetTasks(tasks []string) *GradleCommand {
	gc.tasks = tasks
	return gc
}

func (gc *GradleCommand) SetThreads(threads int) *GradleCommand {
	gc.threads = threads
	return gc
}

func (gc *GradleCommand) SetDetailedSummary(detailedSummary bool) *GradleCommand {
	gc.detailedSummary = detailedSummary
	return gc
}

func (gc *GradleCommand) IsDetailedSummary() bool {
	return gc.detailedSummary
}

func (gc *GradleCommand) SetXrayScan(xrayScan bool) *GradleCommand {
	gc.xrayScan = xrayScan
	return gc
}

func (gc *GradleCommand) IsXrayScan() bool {
	return gc.xrayScan
}

func (gc *GradleCommand) SetScanOutputFormat(format format.OutputFormat) *GradleCommand {
	gc.scanOutputFormat = format
	return gc
}

func (gc *GradleCommand) Result() *commandsutils.Result {
	return gc.result
}

func (gc *GradleCommand) setResult(result *commandsutils.Result) *GradleCommand {
	gc.result = result
	return gc
}

// getBuildNameAndNumber returns the build name and build number from the configuration
func (gc *GradleCommand) getBuildNameAndNumber() (buildName, buildNumber string, err error) {
	buildName, err = gc.configuration.GetBuildName()
	if err != nil {
		return
	}
	buildNumber, err = gc.configuration.GetBuildNumber()
	return
}

type InitScriptAuthConfig struct {
	ArtifactoryURL         string
	GradleRepoName         string
	ArtifactoryUsername    string
	ArtifactoryAccessToken string
}

// GenerateInitScript generates a Gradle init script with the provided authentication configuration.
func GenerateInitScript(config InitScriptAuthConfig) (string, error) {
	tmpl, err := template.New("gradleTemplate").Parse(gradleInitScript)
	if err != nil {
		return "", fmt.Errorf("failed to parse Gradle init script template: %s", err)
	}

	// Remove possible trailing slashes from the Artifactory URL to avoid double slashes in the generated script
	config.ArtifactoryURL = strings.TrimSuffix(config.ArtifactoryURL, "/")
	var result strings.Builder
	// Create a string from the template with the provided configuration
	err = tmpl.Execute(&result, config)
	if err != nil {
		return "", fmt.Errorf("failed to write auth configuration into the init script template: %s", err)
	}

	return result.String(), nil
}

// WriteInitScript writes the Gradle init script to the Gradle user home `init.d` directory,
// which stores initialization scripts. The final path should be `$GRADLE_USER_HOME/init.d/jfrog.init.gradle`.
// More info on how Gradle invokes these init scripts can be found here:
// https://docs.gradle.org/current/userguide/init_scripts.html#sec:using_an_init_script
func WriteInitScript(initScript string) error {
	gradleHome := os.Getenv(UserHomeEnv)
	if gradleHome == "" {
		gradleHome = filepath.Join(clientutils.GetUserHomeDir(), ".gradle")
	}
	// Sanitize the path to prevent directory traversal attacks
	gradleHome = filepath.Clean(gradleHome)

	initScriptsDir := filepath.Clean(filepath.Join(gradleHome, "init.d"))
	if err := os.MkdirAll(initScriptsDir, 0755); err != nil {
		return fmt.Errorf("failed to create Gradle init.d directory: %w", err)
	}
	jfrogInitScriptPath := filepath.Clean(filepath.Join(initScriptsDir, InitScriptName))
	if err := os.WriteFile(jfrogInitScriptPath, []byte(initScript), 0644); err != nil {
		return fmt.Errorf("failed to write Gradle init script to %s: %w", jfrogInitScriptPath, err)
	}
	return nil
}

func runGradle(vConfig *viper.Viper, tasks []string, deployableArtifactsFile string, configuration *build.BuildConfiguration, threads int, disableDeploy bool) error {
	buildInfoService := build.CreateBuildInfoService()
	buildName, err := configuration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := configuration.GetBuildNumber()
	if err != nil {
		return err
	}
	gradleBuild, err := buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, configuration.GetProject())
	if err != nil {
		return errorutils.CheckError(err)
	}
	gradleModule, err := gradleBuild.AddGradleModule("")
	if err != nil {
		return errorutils.CheckError(err)
	}
	props, wrapper, plugin, err := createGradleRunConfig(vConfig, deployableArtifactsFile, threads, disableDeploy)
	if err != nil {
		return err
	}
	dependencyLocalPath, err := getGradleDependencyLocalPath()
	if err != nil {
		return err
	}
	gradleModule.SetExtractorDetails(dependencyLocalPath, filepath.Join(coreutils.GetCliPersistentTempDirPath(), build.PropertiesTempPath), tasks, wrapper, plugin, dependencies.DownloadExtractor, props)
	return coreutils.ConvertExitCodeError(gradleModule.CalcDependencies())
}

func getGradleDependencyLocalPath() (string, error) {
	dependenciesPath, err := config.GetJfrogDependenciesPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dependenciesPath, "gradle"), nil
}

func createGradleRunConfig(vConfig *viper.Viper, deployableArtifactsFile string, threads int, disableDeploy bool) (props map[string]string, wrapper, plugin bool, err error) {
	wrapper = vConfig.GetBool(useWrapper)
	if threads > 0 {
		vConfig.Set(build.ForkCount, threads)
	}

	if disableDeploy {
		setDeployFalse(vConfig)
	}
	props, err = build.CreateBuildInfoProps(deployableArtifactsFile, vConfig, project.Gradle)
	if err != nil {
		return
	}
	if deployableArtifactsFile != "" {
		// Save the path to a temp file, where buildinfo project will write the deployable artifacts details.
		props[build.DeployableArtifacts] = fmt.Sprint(vConfig.Get(build.DeployableArtifacts))
	}
	plugin = vConfig.GetBool(usePlugin)
	return
}

func setDeployFalse(vConfig *viper.Viper) {
	vConfig.Set(build.DeployerPrefix+build.DeployArtifacts, "false")
	if vConfig.GetString(build.DeployerPrefix+build.Url) == "" {
		vConfig.Set(build.DeployerPrefix+build.Url, "https://empty_url")
	}
	if vConfig.GetString(build.DeployerPrefix+build.Repo) == "" {
		vConfig.Set(build.DeployerPrefix+build.Repo, "empty_repo")
	}
}
