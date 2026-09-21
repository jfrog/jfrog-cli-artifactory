package mvn

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/flexpack"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"

	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/dependencies"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/spf13/viper"
)

type MvnUtils struct {
	vConfig                   *viper.Viper
	configPath                string
	buildConf                 *buildUtils.BuildConfiguration
	buildArtifactsDetailsFile string
	buildInfoFilePath         string
	goals                     []string
	threads                   int
	insecureTls               bool
	disableDeploy             bool
	outputWriter              io.Writer
	preferWrapper             bool
	// serverDetails is the resolved server (from --server-id, else default) used for native build-info
	// collection - property tagging, virtual-repo resolution and repository lookups.
	serverDetails *config.ServerDetails
}

func NewMvnUtils() *MvnUtils {
	return &MvnUtils{buildConf: &buildUtils.BuildConfiguration{}}
}

func (mu *MvnUtils) SetConfigPath(configPath string) *MvnUtils {
	mu.configPath = configPath
	return mu
}

func (mu *MvnUtils) SetBuildConf(buildConf *buildUtils.BuildConfiguration) *MvnUtils {
	mu.buildConf = buildConf
	return mu
}

func (mu *MvnUtils) SetBuildArtifactsDetailsFile(buildArtifactsDetailsFile string) *MvnUtils {
	mu.buildArtifactsDetailsFile = buildArtifactsDetailsFile
	return mu
}

func (mu *MvnUtils) SetGoals(goals []string) *MvnUtils {
	mu.goals = goals
	return mu
}

func (mu *MvnUtils) SetThreads(threads int) *MvnUtils {
	mu.threads = threads
	return mu
}

func (mu *MvnUtils) SetInsecureTls(insecureTls bool) *MvnUtils {
	mu.insecureTls = insecureTls
	return mu
}

func (mu *MvnUtils) SetDisableDeploy(disableDeploy bool) *MvnUtils {
	mu.disableDeploy = disableDeploy
	return mu
}

func (mu *MvnUtils) SetConfig(vConfig *viper.Viper) *MvnUtils {
	mu.vConfig = vConfig
	return mu
}

func (mu *MvnUtils) SetOutputWriter(writer io.Writer) *MvnUtils {
	mu.outputWriter = writer
	return mu
}

func (mu *MvnUtils) SetServerDetails(serverDetails *config.ServerDetails) *MvnUtils {
	mu.serverDetails = serverDetails
	return mu
}

// SetPreferWrapper controls Maven executable resolution in native (FlexPack) mode.
// When true (jf mvnw), a Maven Wrapper (mvnw/mvnw.cmd) must be present in the working
// directory or a parent; the command fails rather than falling back to PATH "mvn".
// When false (jf mvn), native mode always uses "mvn" from PATH.
func (mu *MvnUtils) SetPreferWrapper(preferWrapper bool) *MvnUtils {
	mu.preferWrapper = preferWrapper
	return mu
}

// resolveMavenExecutable determines which Maven executable native (FlexPack) mode should run.
// jf mvn (preferWrapper=false) always uses "mvn" from PATH.
// jf mvnw (preferWrapper=true) searches upward for the wrapper script itself (mvnw/mvnw.cmd);
// it fails rather than silently falling back to PATH "mvn".
func resolveMavenExecutable(preferWrapper bool) (string, error) {
	if !preferWrapper {
		return "mvn", nil
	}
	wrapperName := "mvnw"
	if coreutils.IsWindows() {
		wrapperName = "mvnw.cmd"
	}
	wrapperDir, exists, err := fileutils.FindUpstream(wrapperName, fileutils.Any)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	if exists {
		return filepath.Join(wrapperDir, wrapperName), nil
	}
	return "", errorutils.CheckErrorf("mvnw invoked but no Maven Wrapper (%s) was found in the current directory or any parent directory", wrapperName)
}

func RunMvn(mu *MvnUtils) error {
	// FlexPack completely bypasses traditional Maven Build Info Extractor
	if utils.ShouldRunNative(mu.configPath) {
		log.Debug("Maven native implementation activated")
		mavenExecutable, err := resolveMavenExecutable(mu.preferWrapper)
		if err != nil {
			return err
		}
		// Execute native Maven command directly (no JFrog Maven plugin)
		cmd := exec.Command(mavenExecutable, mu.goals...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			log.Error("Failed to execute package manager command: " + err.Error())
			return errorutils.CheckError(err)
		}

		// Collect build info if build configuration is provided
		if mu.buildConf != nil {
			isCollectedBuildInfo, err := mu.buildConf.IsCollectBuildInfo()
			if err != nil {
				return err
			}
			if isCollectedBuildInfo {
				log.Info("Collecting build info for executed command...")

				buildName, err := mu.buildConf.GetBuildName()
				if err != nil {
					return err
				}
				buildNumber, err := mu.buildConf.GetBuildNumber()
				if err != nil {
					return err
				}

				// Get working directory
				workingDir, err := os.Getwd()
				if err != nil {
					return errorutils.CheckError(err)
				}

				// Use FlexPack to collect Maven build info. The user's goals/flags are forwarded so the
				// internal dependency resolution matches the profiles/settings the build ran with.
				err = flexpack.CollectMavenBuildInfoWithFlexPack(workingDir, buildName, buildNumber, mu.buildConf, mu.goals, mu.serverDetails)
				if err != nil {
					return errorutils.CheckError(err)
				}
			}
		}

		log.Info("Maven build completed successfully")
		return nil
	}

	buildInfoService := buildUtils.CreateBuildInfoService()
	buildName, err := mu.buildConf.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := mu.buildConf.GetBuildNumber()
	if err != nil {
		return err
	}
	mvnBuild, err := buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, mu.buildConf.GetProject())
	if err != nil {
		return errorutils.CheckError(err)
	}
	mavenModule, err := mvnBuild.AddMavenModule("")
	if err != nil {
		return errorutils.CheckError(err)
	}
	props, useWrapper, err := createMvnRunProps(mu.vConfig, mu.buildArtifactsDetailsFile, mu.threads, mu.insecureTls, mu.disableDeploy)
	if err != nil {
		return err
	}
	var mvnOpts []string
	if v := os.Getenv("MAVEN_OPTS"); v != "" {
		mvnOpts = strings.Fields(v)
	}
	if v, ok := props["buildInfoConfig.artifactoryResolutionEnabled"]; ok {
		mvnOpts = append(mvnOpts, "-DbuildInfoConfig.artifactoryResolutionEnabled="+v)
	}
	projectRoot, exists, err := fileutils.FindUpstream(".mvn", fileutils.Dir)
	if err != nil {
		return errorutils.CheckError(err)
	}
	if !exists {
		projectRoot = ""
	}
	dependencyLocalPath, err := getMavenDependencyLocalPath()
	if err != nil {
		return err
	}
	mavenModule.SetExtractorDetails(dependencyLocalPath,
		filepath.Join(coreutils.GetCliPersistentTempDirPath(), buildUtils.PropertiesTempPath),
		mu.goals,
		dependencies.DownloadExtractor,
		props,
		useWrapper).
		SetOutputWriter(mu.outputWriter)
	mavenModule.SetMavenOpts(mvnOpts...)
	mavenModule.SetRootProjectDir(projectRoot)
	if err = coreutils.ConvertExitCodeError(mavenModule.CalcDependencies()); err != nil {
		return err
	}
	mu.buildInfoFilePath = mavenModule.GetGeneratedBuildInfoPath()
	// Mark the legacy build-info-extractor path so the published JSON is distinguishable from native
	// FlexPack (which stamps the same property with "native"). Best-effort: never fail the build for it.
	stampMavenBuildMode(mu.buildInfoFilePath, entities.MavenBuildModeLegacy)
	return nil
}

// stampMavenBuildMode injects the Maven build-mode marker (entities.MavenBuildModeProperty) into a
// build-info JSON file generated by the legacy extractor, matching what the native FlexPack collector
// records in-process. It edits the raw JSON object so every field the extractor wrote is preserved
// verbatim. The marker is informational, so any failure is logged at debug level and ignored.
func stampMavenBuildMode(buildInfoPath, mode string) {
	if buildInfoPath == "" {
		return
	}
	content, err := os.ReadFile(buildInfoPath)
	if err != nil {
		log.Debug("Skipping maven build-mode stamp, could not read build info: " + err.Error())
		return
	}
	var raw map[string]interface{}
	if err = json.Unmarshal(content, &raw); err != nil {
		log.Debug("Skipping maven build-mode stamp, could not parse build info: " + err.Error())
		return
	}
	props, ok := raw["properties"].(map[string]interface{})
	if !ok || props == nil {
		props = map[string]interface{}{}
		raw["properties"] = props
	}
	props[entities.MavenBuildModeProperty] = mode
	updated, err := json.Marshal(raw)
	if err != nil {
		log.Debug("Skipping maven build-mode stamp, could not serialize build info: " + err.Error())
		return
	}
	tmpFile, tmpErr := os.CreateTemp(filepath.Dir(buildInfoPath), "buildinfo-*.json")
	if tmpErr != nil {
		log.Debug("Skipping maven build-mode stamp, could not create temp file: " + tmpErr.Error())
		return
	}
	tmpPath := tmpFile.Name()
	_, writeErr := tmpFile.Write(updated)
	closeErr := tmpFile.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		log.Debug("Skipping maven build-mode stamp, could not write temp file")
		return
	}
	if err = os.Rename(tmpPath, buildInfoPath); err != nil {
		_ = os.Remove(tmpPath)
		log.Debug("Skipping maven build-mode stamp, could not rename temp file: " + err.Error())
	}
}

// GetBuildInfoFilePath returns the path to the temporary build info file
// This file stores build-info details and is populated by the Maven extractor after CalcDependencies() is called
func (mu *MvnUtils) GetBuildInfoFilePath() string {
	return mu.buildInfoFilePath
}

func getMavenDependencyLocalPath() (string, error) {
	dependenciesPath, err := config.GetJfrogDependenciesPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dependenciesPath, "maven", build.MavenExtractorDependencyVersion), nil
}

func createMvnRunProps(vConfig *viper.Viper, buildArtifactsDetailsFile string, threads int, insecureTls, disableDeploy bool) (props map[string]string, useWrapper bool, err error) {
	useWrapper = vConfig.GetBool("useWrapper")
	vConfig.Set(buildUtils.InsecureTls, insecureTls)
	if threads > 0 {
		vConfig.Set(buildUtils.ForkCount, threads)
	}

	if disableDeploy {
		setDeployFalse(vConfig)
	}

	if vConfig.IsSet("resolver") {
		vConfig.Set("buildInfoConfig.artifactoryResolutionEnabled", "true")
	}

	// Set CI VCS properties if in CI environment
	workingDir, wdErr := os.Getwd()
	if wdErr != nil {
		workingDir = "."
	}
	civcs.SetCIVcsPropsToConfig(vConfig, workingDir)

	buildInfoProps, err := buildUtils.CreateBuildInfoProps(buildArtifactsDetailsFile, vConfig, project.Maven)
	if err != nil {
		return nil, useWrapper, err
	}

	// Set publish.add.deployable.artifacts based on the scenario:
	// - mvn verify/compile/package (disableDeploy=true, no buildArtifactsDetailsFile): false (preserve fix)
	// - mvn deploy/install (disableDeploy=false): true (need deployable artifacts)
	// - Conditional upload (disableDeploy=true, buildArtifactsDetailsFile set): true (for XRay scan)
	if disableDeploy && buildArtifactsDetailsFile == "" {
		// Non-deployment goals (verify, compile, package) - preserve mvn verify fix
		buildInfoProps["publish.add.deployable.artifacts"] = "false"
		log.Debug("Artifact deployment disabled for non-deployment Maven goals")
	} else {
		// Deployment goals (deploy, install) or conditional upload - need deployable artifacts
		buildInfoProps["publish.add.deployable.artifacts"] = "true"
		log.Debug("Artifact deployment enabled for Maven deployment or conditional upload")
	}

	return buildInfoProps, useWrapper, nil
}

func setDeployFalse(vConfig *viper.Viper) {
	vConfig.Set(buildUtils.DeployerPrefix+buildUtils.DeployArtifacts, "false")
	if vConfig.GetString(buildUtils.DeployerPrefix+buildUtils.Url) == "" {
		vConfig.Set(buildUtils.DeployerPrefix+buildUtils.Url, "https://empty_url")
	}
	if vConfig.GetString(buildUtils.DeployerPrefix+buildUtils.ReleaseRepo) == "" {
		vConfig.Set(buildUtils.DeployerPrefix+buildUtils.ReleaseRepo, "empty_repo")
	}
	if vConfig.GetString(buildUtils.DeployerPrefix+buildUtils.SnapshotRepo) == "" {
		vConfig.Set(buildUtils.DeployerPrefix+buildUtils.SnapshotRepo, "empty_repo")
	}
}
