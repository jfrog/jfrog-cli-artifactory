package buildinfo

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/formats"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	biconf "github.com/jfrog/jfrog-client-go/artifactory/buildinfo"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	artclientutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type BuildPublishCommand struct {
	buildConfiguration *build.BuildConfiguration
	serverDetails      *config.ServerDetails
	config             *biconf.Configuration
	detailedSummary    bool
	summary            *clientutils.Sha256Summary
	collectGitInfo     bool
	collectEnv         bool
	BuildAddGitCommand
}

func NewBuildPublishCommand() *BuildPublishCommand {
	return &BuildPublishCommand{}
}

func (bpc *BuildPublishCommand) SetConfig(config *biconf.Configuration) *BuildPublishCommand {
	bpc.config = config
	return bpc
}

func (bpc *BuildPublishCommand) SetServerDetails(serverDetails *config.ServerDetails) *BuildPublishCommand {
	bpc.serverDetails = serverDetails
	return bpc
}

func (bpc *BuildPublishCommand) SetBuildConfiguration(buildConfiguration *build.BuildConfiguration) *BuildPublishCommand {
	bpc.buildConfiguration = buildConfiguration
	return bpc
}

func (bpc *BuildPublishCommand) SetSummary(summary *clientutils.Sha256Summary) *BuildPublishCommand {
	bpc.summary = summary
	return bpc
}

func (bpc *BuildPublishCommand) GetSummary() *clientutils.Sha256Summary {
	return bpc.summary
}

func (bpc *BuildPublishCommand) SetDetailedSummary(detailedSummary bool) *BuildPublishCommand {
	bpc.detailedSummary = detailedSummary
	return bpc
}

func (bpc *BuildPublishCommand) IsDetailedSummary() bool {
	return bpc.detailedSummary
}

func (bpc *BuildPublishCommand) CommandName() string {
	autoPublishedTriggered, err := clientutils.GetBoolEnvValue(coreutils.UsageAutoPublishedBuild, false)
	if err != nil {
		log.Warn("Failed to get the value of the environment variable: " + coreutils.UsageAutoPublishedBuild + ". " + err.Error())
	}
	if autoPublishedTriggered {
		return "rt_build_publish_auto"
	}
	return "rt_build_publish"
}

func (bpc *BuildPublishCommand) CollectGitInfo() bool {
	return bpc.collectGitInfo
}

func (bpc *BuildPublishCommand) SetCollectGitInfo(collectGitInfo bool) *BuildPublishCommand {
	bpc.collectGitInfo = collectGitInfo
	return bpc
}

func (bpc *BuildPublishCommand) CollectEnv() bool {
	return bpc.collectEnv
}

func (bpc *BuildPublishCommand) SetCollectEnv(collectEnv bool) *BuildPublishCommand {
	bpc.collectEnv = collectEnv
	return bpc
}

func (bpc *BuildPublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return bpc.serverDetails, nil
}

func (bpc *BuildPublishCommand) Run() error {
	servicesManager, err := utils.CreateServiceManager(bpc.serverDetails, -1, 0, bpc.config.DryRun)
	if err != nil {
		return err
	}

	buildInfoService := build.CreateBuildInfoService()
	buildName, err := bpc.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := bpc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	// add build related information from git
	if bpc.CollectGitInfo() {
		buildAddGitConfigurationCmd := NewBuildAddGitCommand().SetBuildConfiguration(bpc.buildConfiguration).SetConfigFilePath(bpc.configFilePath).SetServerId(bpc.serverDetails.ServerId)
		if bpc.dotGitPath != "" {
			buildAddGitConfigurationCmd.SetDotGitPath(bpc.dotGitPath)
		}
		err = buildAddGitConfigurationCmd.Run()
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to collect git information for build '%s/%s': %v", buildName, buildNumber, err))
		}
		log.Info("Collected git information.")
	}

	// add environment variables to build info
	if bpc.CollectEnv() {
		buildCollectEnvCmd := NewBuildCollectEnvCommand().SetBuildConfiguration(bpc.buildConfiguration)
		err = buildCollectEnvCmd.Run()
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to collect environment variables for build '%s/%s': %v", buildName, buildNumber, err))
		}
	}

	build, err := buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, bpc.buildConfiguration.GetProject())
	if errorutils.CheckError(err) != nil {
		return err
	}

	build.SetAgentName(coreutils.GetCliUserAgentName())
	build.SetAgentVersion(coreutils.GetCliUserAgentVersion())
	build.SetBuildAgentVersion(coreutils.GetClientAgentVersion())
	build.SetPrincipal(bpc.serverDetails.User)
	build.SetBuildUrl(bpc.config.BuildUrl)

	buildInfo, err := build.ToBuildInfo()
	if errorutils.CheckError(err) != nil {
		return err
	}
	err = buildInfo.IncludeEnv(strings.Split(bpc.config.EnvInclude, ";")...)
	if errorutils.CheckError(err) != nil {
		return err
	}
	err = buildInfo.ExcludeEnv(strings.Split(bpc.config.EnvExclude, ";")...)
	if errorutils.CheckError(err) != nil {
		return err
	}
	if bpc.buildConfiguration.IsLoadedFromConfigFile() {
		buildInfo.Number, err = bpc.getNextBuildNumber(buildInfo.Name, servicesManager)
		if errorutils.CheckError(err) != nil {
			return err
		}
		bpc.buildConfiguration.SetBuildNumber(buildInfo.Number)
	}
	if bpc.config.Overwrite {
		project := bpc.buildConfiguration.GetProject()
		buildRuns, found, err := servicesManager.GetBuildRuns(services.BuildInfoParams{BuildName: buildName, ProjectKey: project})
		if err != nil {
			return err
		}
		if found {
			buildNumbersFrequency := CalculateBuildNumberFrequency(buildRuns)
			if frequency, ok := buildNumbersFrequency[buildNumber]; ok {
				err = servicesManager.DeleteBuildInfo(buildInfo, project, frequency)
				if err != nil {
					return err
				}
			}
		}
	}
	summary, err := servicesManager.PublishBuildInfo(buildInfo, bpc.buildConfiguration.GetProject())
	if bpc.IsDetailedSummary() {
		bpc.SetSummary(summary)
	}
	if err != nil || bpc.config.DryRun {
		return err
	}

	// Set CI VCS properties on artifacts from build info.
	// This only runs if we're in a supported CI environment (GitHub Actions, GitLab CI, etc.)
	// Note: This never returns an error - it only logs warnings on failure
	bpc.setCIVcsPropsOnArtifacts(servicesManager, buildInfo)

	majorVersion, err := utils.GetRtMajorVersion(servicesManager)
	if err != nil {
		return err
	}

	buildLink, err := bpc.constructBuildInfoUiUrl(majorVersion, buildInfo.Started)
	if err != nil {
		return err
	}

	err = build.Clean()
	if err != nil {
		return err
	}

	if err = recordCommandSummary(buildInfo, buildLink); err != nil {
		return err
	}

	logMsg := "Build info successfully deployed."
	if bpc.IsDetailedSummary() {
		log.Info(logMsg + " Browse it in Artifactory under " + buildLink)
		return nil
	}

	log.Info(logMsg)
	return logJsonOutput(buildLink)
}

// CalculateBuildNumberFrequency since the build number is not unique, we need to calculate the frequency of each build number
// in order to delete the correct number of builds and then publish the new build.
func CalculateBuildNumberFrequency(runs *buildinfo.BuildRuns) map[string]int {
	frequency := make(map[string]int)
	for _, run := range runs.BuildsNumbers {
		buildNumber := strings.TrimPrefix(run.Uri, "/")
		frequency[buildNumber]++
	}
	return frequency
}

func logJsonOutput(buildInfoUiUrl string) error {
	output := formats.BuildPublishOutput{BuildInfoUiUrl: buildInfoUiUrl}
	results, err := output.JSON()
	if err != nil {
		return errorutils.CheckError(err)
	}
	log.Output(clientutils.IndentJson(results))
	return nil
}

func (bpc *BuildPublishCommand) constructBuildInfoUiUrl(majorVersion int, buildInfoStarted string) (string, error) {
	buildTime, err := time.Parse(buildinfo.TimeFormat, buildInfoStarted)
	if errorutils.CheckError(err) != nil {
		return "", err
	}
	return bpc.getBuildInfoUiUrl(majorVersion, buildTime)
}

func (bpc *BuildPublishCommand) getBuildInfoUiUrl(majorVersion int, buildTime time.Time) (string, error) {
	buildName, err := bpc.buildConfiguration.GetBuildName()
	if err != nil {
		return "", err
	}
	buildNumber, err := bpc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return "", err
	}

	baseUrl := bpc.serverDetails.GetUrl()
	if baseUrl == "" {
		baseUrl = strings.TrimSuffix(strings.TrimSuffix(bpc.serverDetails.GetArtifactoryUrl(), "/"), "artifactory")
	}
	baseUrl = clientutils.AddTrailingSlashIfNeeded(baseUrl)

	project := bpc.buildConfiguration.GetProject()
	buildName, buildNumber, project = url.PathEscape(buildName), url.PathEscape(buildNumber), url.QueryEscape(project)

	if majorVersion <= 6 {
		return fmt.Sprintf("%vartifactory/webapp/#/builds/%v/%v",
			baseUrl, buildName, buildNumber), nil
	}
	timestamp := buildTime.UnixMilli()
	if project != "" {
		return fmt.Sprintf("%vui/builds/%v/%v/%v/published?buildRepo=%v-build-info&projectKey=%v",
			baseUrl, buildName, buildNumber, strconv.FormatInt(timestamp, 10), project, project), nil
	}
	return fmt.Sprintf("%vui/builds/%v/%v/%v/published?buildRepo=artifactory-build-info",
		baseUrl, buildName, buildNumber, strconv.FormatInt(timestamp, 10)), nil
}

// Return the next build number based on the previously published build.
// Return "1" if no build is found
func (bpc *BuildPublishCommand) getNextBuildNumber(buildName string, servicesManager artifactory.ArtifactoryServicesManager) (string, error) {
	publishedBuildInfo, found, err := servicesManager.GetBuildInfo(services.BuildInfoParams{BuildName: buildName, BuildNumber: artclientutils.LatestBuildNumberKey})
	if err != nil {
		return "", err
	}
	if !found || publishedBuildInfo.BuildInfo.Number == "" {
		return "1", nil
	}
	latestBuildNumber, err := strconv.Atoi(publishedBuildInfo.BuildInfo.Number)
	if errorutils.CheckError(err) != nil {
		if errors.Is(err, strconv.ErrSyntax) {
			log.Warn("The latest build number is " + publishedBuildInfo.BuildInfo.Number + ". Since it is not an integer, and therefore cannot be incremented to automatically generate the next build number, setting the next build number to 1.")
			return "1", nil
		}
		return "", err
	}
	latestBuildNumber++
	return strconv.Itoa(latestBuildNumber), nil
}

// setCIVcsPropsOnArtifacts sets CI VCS properties on all artifacts in the build info.
// This method:
// - Only runs when in a supported CI environment (GitHub Actions, GitLab CI, etc.)
// - Never fails the build publish - only logs warnings on errors
// - Retries transient failures but not 404 errors
// - Does nothing if CI VCS props collection is disabled via JFROG_CLI_CI_VCS_PROPS_DISABLED
func (bpc *BuildPublishCommand) setCIVcsPropsOnArtifacts(
	servicesManager artifactory.ArtifactoryServicesManager,
	buildInfo *buildinfo.BuildInfo,
) {
	// Check if CI VCS props collection is disabled
	if civcs.IsCIVcsPropsDisabled() {
		return
	}
	// Check if running in a supported CI environment
	// This requires CI=true AND a registered provider (GitHub, GitLab, etc.)
	ciVcsInfo := cienv.GetCIVcsInfo()
	if ciVcsInfo.IsEmpty() {
		// Not in CI or no registered provider - silently skip
		return
	}
	log.Debug("CI VCS: Detected provider:", ciVcsInfo.Provider, ", org:", ciVcsInfo.Org, ", repo:", ciVcsInfo.Repo)

	// Build props string
	props := civcs.BuildCIVcsPropsString(ciVcsInfo)
	if props == "" {
		log.Debug("CI VCS: Empty props string, skipping")
		return
	}

	// Extract artifact paths from build info (with warnings for missing repo paths)
	artifactPaths, skippedCount := extractArtifactPathsWithWarnings(buildInfo)
	log.Debug("CI VCS: Extracted", len(artifactPaths), "artifact paths,", skippedCount, "skipped")
	if len(artifactPaths) == 0 && skippedCount == 0 {
		log.Debug("CI VCS: No artifacts found in build info")
		return
	}
	if len(artifactPaths) == 0 {
		// All artifacts were skipped due to missing repo paths
		log.Debug("CI VCS: All artifacts skipped due to missing repo paths")
		return
	}
	log.Debug("CI VCS: Setting properties on", len(artifactPaths), "artifacts with props:", props)
	// Set properties on all artifacts in a single batch call
	setPropsOnArtifacts(servicesManager, artifactPaths, props)
	log.Debug("CI VCS: Property setting completed")
}

func recordCommandSummary(buildInfo *buildinfo.BuildInfo, buildLink string) (err error) {
	if !commandsummary.ShouldRecordSummary() {
		return
	}
	buildInfo.BuildUrl = buildLink
	buildInfoSummary, err := commandsummary.NewBuildInfoSummary()
	if err != nil {
		return
	}
	return buildInfoSummary.Record(buildInfo)
}
