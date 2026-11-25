package buildinfo

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
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
	depExcludeScopes   []string
	// buildInfoUiUrl holds the Artifactory UI URL of the published build info.
	// It is populated by Run() and exposed via GetBuildInfoUiUrl() so the CLI
	// layer can render it when --format is set.
	buildInfoUiUrl string
	// suppressOutput prevents Run() from printing the buildInfoUiUrl JSON to
	// stdout.  Set this to true when the CLI layer will handle output formatting.
	suppressOutput bool
	// collectSha256 causes Run() to store the Sha256Summary even when
	// detailedSummary is false, so the CLI format layer can include the sha256.
	collectSha256 bool
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

// GetBuildInfoUiUrl returns the Artifactory UI URL for the published build info.
// This is populated after Run() completes successfully.
func (bpc *BuildPublishCommand) GetBuildInfoUiUrl() string {
	return bpc.buildInfoUiUrl
}

// SetSuppressOutput instructs Run() not to print the buildInfoUiUrl JSON to
// stdout.  Use this when the CLI layer will handle output formatting.
func (bpc *BuildPublishCommand) SetSuppressOutput(suppress bool) *BuildPublishCommand {
	bpc.suppressOutput = suppress
	return bpc
}

// SetCollectSha256 causes Run() to store the Sha256Summary even when
// detailedSummary is false, so the CLI format layer can include the sha256.
func (bpc *BuildPublishCommand) SetCollectSha256(collect bool) *BuildPublishCommand {
	bpc.collectSha256 = collect
	return bpc
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

func (bpc *BuildPublishCommand) SetDepExcludeScopes(scopes []string) *BuildPublishCommand {
	bpc.depExcludeScopes = scopes
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
	bpc.excludeDependenciesByScope(buildInfo)
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
	if bpc.IsDetailedSummary() || bpc.collectSha256 {
		bpc.SetSummary(summary)
	}
	if err != nil || bpc.config.DryRun {
		return err
	}

	// Set CI VCS properties on artifacts from build info.
	// This only runs if we're in a supported CI environment (GitHub Actions, GitLab CI, etc.)
	// Note: This never returns an error - it only logs warnings on failure
	bpc.setVcsPropsOnArtifacts(servicesManager, buildInfo)

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

	// Always store the URL so the CLI layer can access it for --format rendering.
	bpc.buildInfoUiUrl = buildLink

	logMsg := "Build info successfully deployed."
	if bpc.IsDetailedSummary() {
		log.Info(logMsg + " Browse it in Artifactory under " + buildLink)
		return nil
	}

	log.Info(logMsg)
	if bpc.suppressOutput {
		return nil
	}
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

// setVcsPropsOnArtifacts sets VCS properties on all artifacts in the build info.
// Artifacts with OriginalDeploymentRepo are tagged directly by exact path. When that search returns
// fewer items than requested or its lookup fails (e.g. a Maven SNAPSHOT recorded as -SNAPSHOT but
// stored under a timestamped name), the whole build is resolved through the dedicated build-artifacts
// API as a fallback. Artifacts missing OriginalDeploymentRepo are resolved through the same API; it
// is called at most once per publish. A SetProps failure alone does not trigger the fallback.
// If fewer artifacts were tagged than the build info lists, one warning is logged with the untagged
// count, the likely causes and the last error. Never fails the build publish.
// Note: found and tagged are raw item counts, so a virtual-repo search that returns duplicates, or a
// lookup that returns extra items, can mask a miss.
func (bpc *BuildPublishCommand) setVcsPropsOnArtifacts(
	servicesManager artifactory.ArtifactoryServicesManager,
	buildInfo *buildinfo.BuildInfo,
) {
	if civcs.IsCIVcsPropsDisabled() {
		return
	}
	searchDir := bpc.dotGitPath
	if searchDir == "" {
		var err error
		searchDir, err = os.Getwd()
		if err != nil {
			searchDir = "."
		}
	}
	props := civcs.GetCIVcsPropsString(searchDir)
	if props == "" {
		log.Debug("CI VCS: Empty props string, skipping")
		return
	}
	present, missingCount := extractArtifactPaths(buildInfo)
	log.Debug("CI VCS: Extracted", len(present), "direct paths,", missingCount, "missing OriginalDeploymentRepo")
	total := len(present) + missingCount

	var tagged int
	var lastErr error
	needBuildLookup := missingCount > 0
	// Artifacts WITH OriginalDeploymentRepo: tag directly by exact path.
	if len(present) > 0 {
		outcome := setPropsOnArtifacts(servicesManager, present, props)
		tagged = outcome.tagged
		lastErr = outcome.err
		// Fewer items than requested also covers a failed lookup (nothing found). A SetProps failure
		// with every item found is not a lookup problem, so the build lookup cannot help.
		if outcome.found < len(present) {
			log.Debug("CI VCS: exact-path search found", outcome.found, "of", len(present), "artifacts, falling back to the build-artifacts lookup")
			needBuildLookup = true
		}
	}
	// Artifacts WITHOUT OriginalDeploymentRepo, or missed by the exact-path search: resolve via the
	// dedicated build-artifacts API (one indexed call for the whole build).
	if needBuildLookup {
		outcome := setPropsViaBuildSearch(servicesManager, buildInfo.Name, buildInfo.Number,
			bpc.buildConfiguration.GetProject(), props)
		tagged = max(tagged, outcome.tagged)
		if outcome.err != nil {
			lastErr = outcome.err
		}
	}
	warnIfUntagged(total, tagged, lastErr)
}

func (bpc *BuildPublishCommand) excludeDependenciesByScope(buildInfo *buildinfo.BuildInfo) {
	if len(bpc.depExcludeScopes) == 0 {
		return
	}
	excludeSet := make(map[string]struct{}, len(bpc.depExcludeScopes))
	for _, s := range bpc.depExcludeScopes {
		excludeSet[strings.ToLower(s)] = struct{}{}
	}
	totalExcluded := 0
	for i := range buildInfo.Modules {
		original := buildInfo.Modules[i].Dependencies
		filtered := make([]buildinfo.Dependency, 0, len(original))
		for _, dep := range original {
			if matchesExcludeScope(dep.Scopes, excludeSet) {
				totalExcluded++
			} else {
				filtered = append(filtered, dep)
			}
		}
		buildInfo.Modules[i].Dependencies = filtered
	}
	if totalExcluded > 0 {
		log.Info(fmt.Sprintf("Excluded %d dependencies based on scope filter.", totalExcluded))
	}
}

func matchesExcludeScope(scopes []string, excludeSet map[string]struct{}) bool {
	for _, scope := range scopes {
		if _, found := excludeSet[strings.ToLower(scope)]; found {
			return true
		}
	}
	return false
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
