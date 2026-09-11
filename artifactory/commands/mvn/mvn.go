package mvn

import (
	"encoding/json"
	"os"
	"path"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	artifactoryutils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/spf13/viper"
)

type MvnCommand struct {
	goals              []string
	configPath         string
	insecureTls        bool
	configuration      *build.BuildConfiguration
	serverDetails      *config.ServerDetails
	threads            int
	detailedSummary    bool
	xrayScan           bool
	scanOutputFormat   format.OutputFormat
	result             *commandsutils.Result
	deploymentDisabled bool
	// File path for Maven extractor in which all build's artifacts details will be listed at the end of the build.
	buildArtifactsDetailsFile string
	// Only consulted in native (FlexPack) mode; set by jf mvnw to require a Maven Wrapper.
	preferWrapper bool
}

func NewMvnCommand() *MvnCommand {
	return &MvnCommand{}
}

func (mc *MvnCommand) SetServerDetails(serverDetails *config.ServerDetails) *MvnCommand {
	mc.serverDetails = serverDetails
	return mc
}

func (mc *MvnCommand) SetConfiguration(configuration *build.BuildConfiguration) *MvnCommand {
	mc.configuration = configuration
	return mc
}

func (mc *MvnCommand) SetConfigPath(configPath string) *MvnCommand {
	mc.configPath = configPath
	return mc
}

func (mc *MvnCommand) SetGoals(goals []string) *MvnCommand {
	mc.goals = goals
	return mc
}

func (mc *MvnCommand) SetThreads(threads int) *MvnCommand {
	mc.threads = threads
	return mc
}

func (mc *MvnCommand) SetInsecureTls(insecureTls bool) *MvnCommand {
	mc.insecureTls = insecureTls
	return mc
}

// SetPreferWrapper is only consulted in native (FlexPack) mode. jf mvnw sets this to true,
// requiring a Maven Wrapper (mvnw/mvnw.cmd) to be present; legacy (config-file) mode ignores it.
func (mc *MvnCommand) SetPreferWrapper(preferWrapper bool) *MvnCommand {
	mc.preferWrapper = preferWrapper
	return mc
}

func (mc *MvnCommand) SetDetailedSummary(detailedSummary bool) *MvnCommand {
	mc.detailedSummary = detailedSummary
	return mc
}

func (mc *MvnCommand) IsDetailedSummary() bool {
	return mc.detailedSummary
}

func (mc *MvnCommand) SetXrayScan(xrayScan bool) *MvnCommand {
	mc.xrayScan = xrayScan
	return mc
}

func (mc *MvnCommand) IsXrayScan() bool {
	return mc.xrayScan
}

func (mc *MvnCommand) SetScanOutputFormat(format format.OutputFormat) *MvnCommand {
	mc.scanOutputFormat = format
	return mc
}

func (mc *MvnCommand) Result() *commandsutils.Result {
	return mc.result
}

func (mc *MvnCommand) setResult(result *commandsutils.Result) *MvnCommand {
	mc.result = result
	return mc
}

func (mc *MvnCommand) init() (vConfig *viper.Viper, err error) {
	// Read config
	vConfig, err = build.ReadMavenConfig(mc.configPath, nil)
	if err != nil {
		return
	}
	if mc.IsXrayScan() && !vConfig.IsSet("deployer") {
		err = errorutils.CheckErrorf("Conditional upload can only be performed if deployer is set in the config")
		return
	}
	// Maven's extractor deploys build artifacts. This should be disabled since there is no intent to deploy anything or deploy upon Xray scan results.
	// Deployment is enabled only for "install" and "deploy" goals when deployer is configured.
	mc.deploymentDisabled = mc.IsXrayScan() || !vConfig.IsSet("deployer") || !mc.isDeploymentRequested()

	// Warn if deployer is configured but Maven goal does not trigger deployment
	if vConfig.IsSet("deployer") && !mc.IsXrayScan() && mc.deploymentDisabled {
		log.Warn("Deployer repository is configured but Maven goal does not trigger deployment. Only 'install' and 'deploy' goals (including deploy:deploy-file) will deploy artifacts to Artifactory.")
	}

	if mc.shouldCreateBuildArtifactsFile() {
		// Created a file that will contain all the details about the build's artifacts
		tempFile, err := fileutils.CreateTempFile()
		if err != nil {
			return nil, err
		}
		// If this is a Windows machine there is a need to modify the path for the build info file to match Java syntax with double \\
		mc.buildArtifactsDetailsFile = ioutils.DoubleWinPathSeparator(tempFile.Name())
		if err = tempFile.Close(); errorutils.CheckError(err) != nil {
			return nil, err
		}
	}
	return
}

// isDeploymentRequested checks if the user explicitly requested deployment
// by looking for "install" or "deploy" goals in the Maven command.
// These are the only goals that should trigger artifact deployment to Artifactory.
func (mc *MvnCommand) isDeploymentRequested() bool {
	for _, goal := range mc.goals {
		// Exclude help goals (e.g., deploy:help, maven-deploy-plugin:help)
		if strings.HasSuffix(goal, ":help") || goal == "help" {
			continue
		}

		// Exact match for standard Maven phases (most common case)
		if goal == "install" || goal == "deploy" {
			return true
		}

		// Prefix match for plugin:goal format (e.g., deploy:deploy-file, install:install-file)
		if strings.HasPrefix(goal, "deploy:") || strings.HasPrefix(goal, "install:") {
			return true
		}

		// Suffix match for full plugin name format (e.g., maven-deploy-plugin:deploy, maven-install-plugin:install)
		// Note: Using suffix instead of Contains() to avoid false positives like "uninstall", "reinstall"
		if strings.HasSuffix(goal, ":deploy") || strings.HasSuffix(goal, ":install") {
			return true
		}
	}
	return false
}

// Maven extractor generates the details of the build's artifacts.
// This is required for Xray scan and for the detailed summary.
// We can either scan or print the generated artifacts.
func (mc *MvnCommand) shouldCreateBuildArtifactsFile() bool {
	return (mc.IsDetailedSummary() && !mc.deploymentDisabled) || mc.IsXrayScan()
}

func (mc *MvnCommand) Run() error {
	// Check for FlexPack FIRST, before any config loading
	if artifactoryutils.ShouldRunNative(mc.configPath) {
		// FlexPack completely bypasses traditional Maven - no config needed
		// This is handled in RunMvn() in utils.go
		// Create minimal MvnUtils with only what FlexPack needs
		mvnParams := NewMvnUtils().
			SetConfigPath(mc.configPath).
			SetGoals(mc.goals).
			SetBuildConf(mc.configuration).
			SetServerDetails(mc.serverDetails).
			SetPreferWrapper(mc.preferWrapper)
		return RunMvn(mvnParams)
	}

	vConfig, err := mc.init()
	if err != nil {
		return err
	}

	mvnParams := NewMvnUtils().
		SetConfigPath(mc.configPath).
		SetConfig(vConfig).
		SetBuildArtifactsDetailsFile(mc.buildArtifactsDetailsFile).
		SetBuildConf(mc.configuration).
		SetGoals(mc.goals).
		SetInsecureTls(mc.insecureTls).
		SetDisableDeploy(mc.deploymentDisabled).
		SetThreads(mc.threads)
	if err = RunMvn(mvnParams); err != nil {
		return err
	}

	if mc.configuration != nil {
		isCollectedBuildInfo, err := mc.configuration.IsCollectBuildInfo()
		if err != nil {
			return err
		}
		if isCollectedBuildInfo {
			if err = mc.updateBuildInfoArtifactsWithDeploymentRepo(vConfig, mvnParams.GetBuildInfoFilePath()); err != nil {
				return err
			}
		}
	}

	if mc.buildArtifactsDetailsFile == "" {
		return nil
	}

	if err = mc.unmarshalDeployableArtifacts(mc.buildArtifactsDetailsFile); err != nil {
		return err
	}
	if mc.IsXrayScan() {
		return mc.conditionalUpload(mvnParams.GetBuildInfoFilePath())
	}
	return nil
}

// Returns the ServerDetails. The information returns from the config file provided.
func (mc *MvnCommand) ServerDetails() (*config.ServerDetails, error) {
	// Get the serverDetails from the config file.
	if mc.serverDetails == nil {
		vConfig, err := project.ReadConfigFile(mc.configPath, project.YAML)
		if err != nil {
			return nil, err
		}
		mc.serverDetails, err = build.GetServerDetails(vConfig)
		if err != nil {
			return nil, err
		}
	}
	return mc.serverDetails, nil
}

func (mc *MvnCommand) unmarshalDeployableArtifacts(filesPath string) error {
	result, err := commandsutils.UnmarshalDeployableArtifacts(filesPath, mc.configPath, mc.IsXrayScan())
	if err != nil {
		return err
	}
	mc.setResult(result)
	return nil
}

func (mc *MvnCommand) CommandName() string {
	return "rt_maven"
}

// ConditionalUpload will scan the artifact using Xray and will upload them only if the scan passes with no
// violation.
// If an upload fails, the artifacts that did not reach Artifactory are removed from the generated
// build-info (see removeFailedArtifactsFromBuildInfo). Otherwise the build-info would reference
// artifacts that don't exist in Artifactory, causing downstream build-based flows such as
// `jf rbc` (release-bundle-create) to fail with "Unresolvable build artifact".
func (mc *MvnCommand) conditionalUpload(buildInfoFilePath string) error {
	// Initialize the server details (from config) if it hasn't been initialized yet.
	_, err := mc.ServerDetails()
	if err != nil {
		return err
	}
	binariesSpecFile, pomSpecFile, err := commandsutils.ScanDeployableArtifacts(mc.result, mc.serverDetails, mc.threads, mc.scanOutputFormat)
	// If the detailed summary wasn't requested, the reader should be closed here.
	// (otherwise it will be closed by the detailed summary print method)
	if !mc.IsDetailedSummary() {
		e := mc.result.Reader().Close()
		if e != nil {
			return e
		}
	} else {
		mc.result.Reader().Reset()
	}
	if err != nil {
		return err
	}
	// The case scan failed
	if binariesSpecFile == nil {
		return nil
	}
	// Build-info cleanup on upload failure is only relevant when build-info is being collected.
	// When it isn't, we keep the original, lighter upload behavior (no detailed summary, no cleanup).
	collectBuildInfo := false
	if mc.configuration != nil {
		if collectBuildInfo, err = mc.configuration.IsCollectBuildInfo(); err != nil {
			return err
		}
	}
	// First upload binaries
	if len(binariesSpecFile.Files) > 0 {
		uploadCmd := generic.NewUploadCommand()
		uploadConfiguration := new(utils.UploadConfiguration)
		uploadConfiguration.Threads = mc.threads
		uploadCmd.SetUploadConfiguration(uploadConfiguration).SetBuildConfiguration(mc.configuration).SetSpec(binariesSpecFile).SetServerDetails(mc.serverDetails)
		// When build-info is collected, enable detailed summary so the command retains the list of
		// successfully uploaded files, allowing us to remove only the artifacts that actually failed.
		uploadCmd.SetDetailedSummary(collectBuildInfo)
		if err = uploadCmd.Run(); err != nil {
			if collectBuildInfo {
				// Remove the binaries that were not uploaded successfully. The pom.xml's were not uploaded
				// at all (we return below), so all of them are removed as well. This prevents the build-info
				// from referencing artifacts that are not in Artifactory.
				failedNames := failedUploadArtifactNames(binariesSpecFile, uploadCmd)
				for name := range collectArtifactNames(pomSpecFile) {
					failedNames[name] = true
				}
				mc.removeArtifactsFromBuildInfo(buildInfoFilePath, failedNames)
			}
			return err
		}
		if collectBuildInfo {
			closeUploadResultReader(uploadCmd)
		}
	}
	if len(pomSpecFile.Files) > 0 {
		// Then Upload pom.xml's
		uploadCmd := generic.NewUploadCommand()
		uploadConfiguration := new(utils.UploadConfiguration)
		uploadConfiguration.Threads = mc.threads
		uploadCmd.SetUploadConfiguration(uploadConfiguration).SetBuildConfiguration(mc.configuration).SetSpec(pomSpecFile).SetServerDetails(mc.serverDetails)
		uploadCmd.SetDetailedSummary(collectBuildInfo)
		if err = uploadCmd.Run(); err != nil {
			if collectBuildInfo {
				mc.removeArtifactsFromBuildInfo(buildInfoFilePath, failedUploadArtifactNames(pomSpecFile, uploadCmd))
			}
			return err
		}
		if collectBuildInfo {
			closeUploadResultReader(uploadCmd)
		}
	}
	return nil
}

// failedUploadArtifactNames returns the set of artifact file names from specFile that were NOT
// successfully uploaded by uploadCmd. It reads the upload's transfer-details reader (retained because
// detailed summary is enabled on the command) to determine which files actually reached Artifactory,
// so only the artifacts that truly failed are removed from the build-info. If no transfer details are
// available (for example, the upload failed before any file was processed) every artifact in the spec
// is considered failed.
func failedUploadArtifactNames(specFile *spec.SpecFiles, uploadCmd *generic.UploadCommand) map[string]bool {
	failedNames := collectArtifactNames(specFile)
	reader := uploadCmd.Result().Reader()
	if reader == nil {
		return failedNames
	}
	defer func() {
		if e := reader.Close(); e != nil {
			log.Debug("Failed closing upload transfer-details reader: " + e.Error())
		}
	}()
	reader.Reset()
	for details := new(clientutils.FileTransferDetails); reader.NextRecord(details) == nil; details = new(clientutils.FileTransferDetails) {
		// A file present in the transfer details was uploaded successfully - keep it in the build-info.
		delete(failedNames, path.Base(details.TargetPath))
	}
	if e := reader.GetError(); e != nil {
		log.Debug("Failed reading upload transfer-details: " + e.Error())
	}
	return failedNames
}

// closeUploadResultReader closes the transfer-details reader retained by the upload command (when
// detailed summary is enabled) on the success path, to avoid leaking the backing temp file.
func closeUploadResultReader(uploadCmd *generic.UploadCommand) {
	if reader := uploadCmd.Result().Reader(); reader != nil {
		if e := reader.Close(); e != nil {
			log.Debug("Failed closing upload transfer-details reader: " + e.Error())
		}
	}
}

// removeArtifactsFromBuildInfo removes the artifacts whose names are in failedNames from the generated
// build-info file. Keeping artifacts that failed to upload would leave the build-info referencing files
// that never reached Artifactory, which breaks build-based flows like `jf rbc`.
// This is best-effort: any failure here is logged at debug level and ignored so that it never masks
// the original upload error.
func (mc *MvnCommand) removeArtifactsFromBuildInfo(buildInfoFilePath string, failedNames map[string]bool) {
	if buildInfoFilePath == "" || len(failedNames) == 0 {
		return
	}
	exists, err := fileutils.IsFileExists(buildInfoFilePath, false)
	if err != nil {
		log.Debug("Skipping build-info cleanup, could not access build info file: " + err.Error())
		return
	}
	if !exists {
		return
	}
	content, err := os.ReadFile(buildInfoFilePath)
	if err != nil {
		log.Debug("Skipping build-info cleanup, could not read build info file: " + err.Error())
		return
	}
	if len(content) == 0 {
		return
	}
	buildInfo := new(entities.BuildInfo)
	if err = json.Unmarshal(content, &buildInfo); err != nil {
		log.Debug("Skipping build-info cleanup, could not parse build info file: " + err.Error())
		return
	}
	removed := false
	for moduleIndex := range buildInfo.Modules {
		currModule := &buildInfo.Modules[moduleIndex]
		keptArtifacts := currModule.Artifacts[:0]
		for _, artifact := range currModule.Artifacts {
			if failedNames[artifact.Name] {
				removed = true
				continue
			}
			keptArtifacts = append(keptArtifacts, artifact)
		}
		currModule.Artifacts = keptArtifacts
	}
	if !removed {
		return
	}
	newBuildInfo, err := json.Marshal(buildInfo)
	if err != nil {
		log.Debug("Skipping build-info cleanup, could not serialize build info: " + err.Error())
		return
	}
	if err = os.WriteFile(buildInfoFilePath, newBuildInfo, 0644); err != nil {
		log.Debug("Skipping build-info cleanup, could not write build info file: " + err.Error())
		return
	}
	log.Debug("Removed artifacts that failed to upload from the generated build-info to avoid unresolvable build artifacts.")
}

// collectArtifactNames returns the set of artifact file names (the base name of each spec file's
// target path) contained in the given spec files. These names match the "name" field recorded for
// each artifact in the generated build-info.
func collectArtifactNames(specFiles ...*spec.SpecFiles) map[string]bool {
	names := make(map[string]bool)
	for _, specFile := range specFiles {
		if specFile == nil {
			continue
		}
		for i := 0; i < len(specFile.Files); i++ {
			target := specFile.Get(i).Target
			if target == "" {
				continue
			}
			names[path.Base(target)] = true
		}
	}
	return names
}

// updateBuildInfoArtifactsWithDeploymentRepo updates existing build-info temp file with the target repository for each artifact
func (mc *MvnCommand) updateBuildInfoArtifactsWithDeploymentRepo(vConfig *viper.Viper, buildInfoFilePath string) error {
	exists, err := fileutils.IsFileExists(buildInfoFilePath, false)
	if err != nil || !exists {
		return err
	}
	content, err := os.ReadFile(buildInfoFilePath)
	if err != nil {
		return errorutils.CheckErrorf("failed to read build info file: %s", err.Error())
	}
	// In some scenarios, the build info details are set but no content is generated.
	// For example, running a `jf mvn <command>` from the Setup JFrog Cli GitHub Action
	// automatically fills the build info details, but the command itself may not generate any content.
	// Examples include `mvn -v`, `mvn test`, etc.
	if len(content) == 0 {
		return nil
	}
	buildInfo := new(entities.BuildInfo)
	if err = json.Unmarshal(content, &buildInfo); err != nil {
		return errorutils.CheckErrorf("failed to parse build info file: %s", err.Error())
	}

	if vConfig.IsSet(project.ProjectConfigDeployerPrefix) {
		snapshotRepository := vConfig.GetString(build.DeployerPrefix + build.SnapshotRepo)
		releaseRepository := vConfig.GetString(build.DeployerPrefix + build.ReleaseRepo)
		for moduleIndex := range buildInfo.Modules {
			currModule := &buildInfo.Modules[moduleIndex]
			for artifactIndex := range currModule.Artifacts {
				updateArtifactRepo(&currModule.Artifacts[artifactIndex], snapshotRepository, releaseRepository)
			}
			for artifactIndex := range currModule.ExcludedArtifacts {
				updateArtifactRepo(&currModule.ExcludedArtifacts[artifactIndex], snapshotRepository, releaseRepository)
			}
		}
	}

	newBuildInfo, err := json.Marshal(buildInfo)
	if err != nil {
		return errorutils.CheckErrorf("failed to marshal build info: %s", err.Error())
	}

	return os.WriteFile(buildInfoFilePath, newBuildInfo, 0644)
}

func updateArtifactRepo(artifact *entities.Artifact, snapshotRepo, releaseRepo string) {
	if snapshotRepo != "" && strings.Contains(artifact.Path, "-SNAPSHOT") {
		artifact.OriginalDeploymentRepo = snapshotRepo
	} else {
		artifact.OriginalDeploymentRepo = releaseRepo
	}
}
