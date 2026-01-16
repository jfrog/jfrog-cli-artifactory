package buildinfo

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/formats"
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
	"github.com/jfrog/jfrog-client-go/utils/io/content"
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
func (bpc *BuildPublishCommand) setCIVcsPropsOnArtifacts(
	servicesManager artifactory.ArtifactoryServicesManager,
	buildInfo *buildinfo.BuildInfo,
) {
	// Check if running in a supported CI environment
	// This requires CI=true AND a registered provider (GitHub, GitLab, etc.)
	ciVcsInfo := cienv.GetCIVcsInfo()
	if ciVcsInfo.IsEmpty() {
		// Not in CI or no registered provider - silently skip
		return
	}

	log.Debug(fmt.Sprintf("Detected CI provider: %s, org: %s, repo: %s",
		ciVcsInfo.Provider, ciVcsInfo.Org, ciVcsInfo.Repo))

	// Build props string
	props := buildCIVcsPropsString(ciVcsInfo)
	if props == "" {
		return
	}

	// Extract artifact paths from build info (with warnings for missing repo paths)
	artifactPaths, skippedCount := extractArtifactPathsWithWarnings(buildInfo)
	if len(artifactPaths) == 0 && skippedCount == 0 {
		log.Debug("No artifacts found in build info")
		return
	}

	if len(artifactPaths) == 0 {
		// All artifacts were skipped due to missing repo paths
		return
	}

	log.Info(fmt.Sprintf("Setting CI VCS properties on %d artifacts...", len(artifactPaths)))

	// Set properties on each artifact with retry
	var failedCount int
	var notFoundCount int
	for _, artifactPath := range artifactPaths {
		result := setPropsWithRetry(servicesManager, artifactPath, props)
		switch result {
		case propsResultSuccess:
			// OK
		case propsResultNotFound:
			log.Warn(fmt.Sprintf("Artifact not found: %s", artifactPath))
			notFoundCount++
		case propsResultFailed:
			log.Warn(fmt.Sprintf("Unable to set CI VCS properties on artifact: %s", artifactPath))
			failedCount++
		}
	}

	// Log summary
	successCount := len(artifactPaths) - failedCount - notFoundCount
	if successCount > 0 {
		log.Info(fmt.Sprintf("Successfully set CI VCS properties on %d artifacts", successCount))
	}
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

// extractArtifactPathsWithWarnings extracts full Artifactory paths from build info artifacts.
// Returns the list of valid paths and count of skipped artifacts (missing repo path).
// Logs a warning for each artifact missing OriginalDeploymentRepo.
func extractArtifactPathsWithWarnings(buildInfo *buildinfo.BuildInfo) ([]string, int) {
	var paths []string
	var skippedCount int

	for _, module := range buildInfo.Modules {
		for _, artifact := range module.Artifacts {
			if artifact.OriginalDeploymentRepo == "" {
				// Warn about missing repo path
				artifactIdentifier := artifact.Name
				if artifactIdentifier == "" {
					artifactIdentifier = artifact.Path
				}
				if artifactIdentifier == "" {
					artifactIdentifier = fmt.Sprintf("sha1:%s", artifact.Sha1)
				}
				log.Warn(fmt.Sprintf("Unable to find repo path for artifact: %s", artifactIdentifier))
				skippedCount++
				continue
			}

			fullPath := constructArtifactPath(artifact)
			if fullPath != "" {
				paths = append(paths, fullPath)
			}
		}
	}
	return paths, skippedCount
}

// constructArtifactPath builds the full Artifactory path for an artifact.
func constructArtifactPath(artifact buildinfo.Artifact) string {
	if artifact.OriginalDeploymentRepo == "" {
		return ""
	}
	if artifact.Path != "" {
		return artifact.OriginalDeploymentRepo + "/" + artifact.Path
	}
	if artifact.Name != "" {
		return artifact.OriginalDeploymentRepo + "/" + artifact.Name
	}
	return ""
}

// buildCIVcsPropsString constructs the properties string from CI VCS info.
func buildCIVcsPropsString(info cienv.CIVcsInfo) string {
	var parts []string
	if info.Provider != "" {
		parts = append(parts, "vcs.provider="+info.Provider)
	}
	if info.Org != "" {
		parts = append(parts, "vcs.org="+info.Org)
	}
	if info.Repo != "" {
		parts = append(parts, "vcs.repo="+info.Repo)
	}
	return strings.Join(parts, ";")
}

// propsResult represents the outcome of setting properties on an artifact.
type propsResult int

const (
	propsResultSuccess propsResult = iota
	propsResultNotFound
	propsResultFailed
)

const (
	maxRetries     = 3
	retryDelayBase = time.Second
)

// setPropsWithRetry sets properties on an artifact with retry logic.
// Returns:
// - propsResultSuccess: properties set successfully
// - propsResultNotFound: artifact not found (404)
// - propsResultFailed: failed after all retries
func setPropsWithRetry(
	servicesManager artifactory.ArtifactoryServicesManager,
	artifactPath string,
	props string,
) propsResult {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := retryDelayBase * time.Duration(1<<(attempt-1))
			log.Debug(fmt.Sprintf("Retrying property set for %s (attempt %d/%d) after %v",
				artifactPath, attempt+1, maxRetries, delay))
			time.Sleep(delay)
		}

		// Create reader for single artifact
		reader, err := createSingleArtifactReader(artifactPath)
		if err != nil {
			log.Debug(fmt.Sprintf("Failed to create reader for %s: %v", artifactPath, err))
			return propsResultFailed
		}

		params := services.PropsParams{
			Reader: reader,
			Props:  props,
		}

		_, err = servicesManager.SetProps(params)
		_ = reader.Close()

		if err == nil {
			log.Debug(fmt.Sprintf("Set CI VCS properties on: %s", artifactPath))
			return propsResultSuccess
		}

		// Check if error is 404 - don't retry
		if is404Error(err) {
			return propsResultNotFound
		}

		// Check if error is 403 - limited retries
		if is403Error(err) {
			// 403 might be temporary (token refresh) or permanent (no permission)
			// Retry once, then give up
			if attempt >= 1 {
				log.Debug("403 Forbidden persists, likely permission issue")
				return propsResultFailed
			}
		}

		lastErr = err
		log.Debug(fmt.Sprintf("Attempt %d failed for %s: %v", attempt+1, artifactPath, err))
	}

	// All retries exhausted
	log.Debug(fmt.Sprintf("All %d attempts failed for %s: %v", maxRetries, artifactPath, lastErr))
	return propsResultFailed
}

// is404Error checks if the error indicates a 404 Not Found response.
func is404Error(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "not found")
}

// is403Error checks if the error indicates a 403 Forbidden response.
func is403Error(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "forbidden")
}

// createSingleArtifactReader creates a ContentReader for a single artifact path.
func createSingleArtifactReader(artifactPath string) (*content.ContentReader, error) {
	writer, err := content.NewContentWriter("results", true, false)
	if err != nil {
		return nil, err
	}

	// Parse path into repo/path/name
	parts := strings.SplitN(artifactPath, "/", 2)
	if len(parts) < 2 {
		_ = writer.Close()
		return nil, fmt.Errorf("invalid artifact path: %s", artifactPath)
	}

	repo := parts[0]
	pathAndName := parts[1]
	dir, name := path.Split(pathAndName)

	writer.Write(artclientutils.ResultItem{
		Repo: repo,
		Path: strings.TrimSuffix(dir, "/"),
		Name: name,
		Type: "file",
	})

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return content.NewContentReader(writer.GetFilePath(), "results"), nil
}
