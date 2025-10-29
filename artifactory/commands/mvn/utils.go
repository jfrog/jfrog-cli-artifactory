package mvn

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/flexpack"
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
	buildConf                 *buildUtils.BuildConfiguration
	buildArtifactsDetailsFile string
	buildInfoFilePath         string
	goals                     []string
	threads                   int
	insecureTls               bool
	disableDeploy             bool
	outputWriter              io.Writer
}

func NewMvnUtils() *MvnUtils {
	return &MvnUtils{buildConf: &buildUtils.BuildConfiguration{}}
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

func RunMvn(mu *MvnUtils) error {
	// FlexPack completely bypasses traditional Maven Build Info Extractor
	if os.Getenv("JFROG_RUN_NATIVE") == "true" {
		log.Debug("Maven native implementation activated")
		// Execute native Maven command directly (no JFrog Maven plugin)
		cmd := exec.Command("mvn", mu.goals...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
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

				// Use FlexPack to collect Maven build info
				err = flexpack.CollectMavenBuildInfoWithFlexPack(workingDir, buildName, buildNumber, mu.buildConf)
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
	return nil
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
		vConfig.Set(buildUtils.DeployerPrefix+buildUtils.Url, "http://empty_url")
	}
	if vConfig.GetString(buildUtils.DeployerPrefix+buildUtils.ReleaseRepo) == "" {
		vConfig.Set(buildUtils.DeployerPrefix+buildUtils.ReleaseRepo, "empty_repo")
	}
	if vConfig.GetString(buildUtils.DeployerPrefix+buildUtils.SnapshotRepo) == "" {
		vConfig.Set(buildUtils.DeployerPrefix+buildUtils.SnapshotRepo, "empty_repo")
	}
}
