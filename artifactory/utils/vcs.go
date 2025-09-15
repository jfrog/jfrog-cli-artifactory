package utils

import (
	"errors"
	buildinfo "github.com/jfrog/build-info-go/entities"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	utilsconfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	artclientutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const (
	revisionRangeErrPrefix = "fatal: Invalid revision range"
)

type BuildAndVcsDetails interface {
	ParseGitLogFromLastVcsRevision(gitDetails GitLogDetails, logRegExp *gofrogcmd.CmdOutputPattern, lastVcsRevision string) (err error)
	GetPlainGitLogFromPreviousBuild(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration, gitDetails GitLogDetails) (string, error)
	GetLastBuildLink(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration) (string, error)
}

type GitLogDetails struct {
	LogLimit     int
	PrettyFormat string
	// Optional
	DotGitPath string
}

// ParseGitLogFromLastBuild Parses git commits from the last build's VCS revision.
// Calls git log with a custom format, and parses each line of the output with regexp. logRegExp is used to parse the log lines.
func ParseGitLogFromLastBuild(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration, gitDetails GitLogDetails, logRegExp *gofrogcmd.CmdOutputPattern) error {
	vcsUrl, err := validateGitAndGetVcsUrl(&gitDetails)
	if err != nil {
		return err
	}

	// Get latest build's VCS revision from Artifactory.
	lastVcsRevision, err := getLatestVcsRevision(serverDetails, buildConfiguration, vcsUrl)
	if err != nil {
		return err
	}
	return ParseGitLogFromLastVcsRevision(gitDetails, logRegExp, lastVcsRevision)
}

// GetPlainGitLogFromPreviousBuild Returns the git log output for the VCS revision for the previous build in position previousBuildPos.
// For previousBuildPos 0 the latest build is returned, for an input 1 the latest -1 is returned, etc. previousBuildPos must be 0 or above.
// Calls git log with a custom format, and returns the output as is.
// Return RevisionRangeError if revision isn't found (due to git history modification).
func GetPlainGitLogFromPreviousBuild(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration, gitDetails GitLogDetails) (string, error) {
	vcsUrl, err := validateGitAndGetVcsUrl(&gitDetails)
	if err != nil {
		return "", err
	}

	lastVcsRevision, err := getVcsFromPreviousBuild(serverDetails, buildConfiguration, vcsUrl)
	if err != nil {
		return "", err
	}

	return getPlainGitLogFromLastVcsRevision(gitDetails, lastVcsRevision)
}

func GetLastBuildLink(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration) (string, error) {
	lastPublishedBuildInfo, err := getPreviousBuild(serverDetails, buildConfiguration, 0)
	if err != nil {
		return "", err
	}
	uri, err := convertToUiLink(lastPublishedBuildInfo)
	if err != nil {
		return "", err
	}
	return uri, nil
}

// ParseGitLogFromLastVcsRevision Parses git log line by line, using the parser provided in logRegExp.
// Git log is parsed from lastVcsRevision to HEAD.
func ParseGitLogFromLastVcsRevision(gitDetails GitLogDetails, logRegExp *gofrogcmd.CmdOutputPattern, lastVcsRevision string) (err error) {
	logCmd, cleanupFunc, err := prepareGitLogCommand(gitDetails, lastVcsRevision)
	defer func() {
		if cleanupFunc != nil {
			err = errors.Join(err, cleanupFunc())
		}
	}()

	errRegExp, err := createErrRegExpHandler(lastVcsRevision)
	if err != nil {
		return err
	}

	// Run git command.
	_, _, exitOk, err := gofrogcmd.RunCmdWithOutputParser(logCmd, false, logRegExp, errRegExp)
	if errorutils.CheckError(err) != nil {
		var revisionRangeError RevisionRangeError
		if errors.As(err, &revisionRangeError) {
			// Revision not found in range. Ignore and return.
			log.Info(err.Error())
			return nil
		}
		return err
	}
	if !exitOk {
		// May happen when trying to run git log for non-existing revision.
		err = errorutils.CheckErrorf("failed executing git log command")
	}
	return err
}

// GetDotGit Looks for the .git directory in the current directory and its parents.
func GetDotGit(providedDotGitPath string) (string, error) {
	if providedDotGitPath != "" {
		return providedDotGitPath, nil
	}
	dotGitPath, exists, err := fileutils.FindUpstream(".git", fileutils.Any)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", errorutils.CheckErrorf("Could not find .git")
	}
	return dotGitPath, nil
}

// Gets the vcs revision from the latest build in Artifactory.
func getLatestVcsRevision(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration, vcsUrl string) (string, error) {
	buildInfo, err := getLatestBuildInfo(serverDetails, buildConfiguration)
	if err != nil {
		return "", err
	}

	return getMatchingRevisionFromBuild(buildInfo, vcsUrl), nil
}

// Gets the vcs revision from the build in position "previousBuildPos" in Artifactory. previousBuildPos = 0 is the latest build.
// previousBuildPos must be 0 or larger.
func getVcsFromPreviousBuild(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration, vcsUrl string) (string, error) {
	buildInfo, err := getPreviousBuildsCommit(serverDetails, buildConfiguration)
	if err != nil {
		return "", err
	}

	return getMatchingRevisionFromBuild(&buildInfo.BuildInfo, vcsUrl), nil
}

// Returns the vcs revision that matches th provided vcs url.
func getMatchingRevisionFromBuild(buildInfo *buildinfo.BuildInfo, vcsUrl string) string {
	lastVcsRevision := ""
	for _, vcs := range buildInfo.VcsList {
		if vcs.Url == vcsUrl {
			lastVcsRevision = vcs.Revision
			break
		}
	}
	return lastVcsRevision
}

// Returns build info, or empty build info struct if not found.
func getLatestBuildInfo(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration) (*buildinfo.BuildInfo, error) {
	// Create services manager to get build-info from Artifactory.
	sm, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return nil, err
	}

	// Get latest build-info from Artifactory.
	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return nil, err
	}
	buildInfoParams := services.BuildInfoParams{BuildName: buildName, BuildNumber: artclientutils.LatestBuildNumberKey, ProjectKey: buildConfiguration.GetProject()}
	publishedBuildInfo, found, err := sm.GetBuildInfo(buildInfoParams)
	if err != nil {
		return nil, err
	}
	if !found {
		return &buildinfo.BuildInfo{}, nil
	}

	return &publishedBuildInfo.BuildInfo, nil
}

// Returns the previous build in order provided by previousBuildPos. For previousBuildPos 0 the latest build is returned.
// If previousBuildPos is not 0 or above, a general error will be returned.
// If the build does not exist, or there are less previous build runs than requested, an empty build will be returned.
func getPreviousBuild(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration, previousBuildPos int) (*buildinfo.PublishedBuildInfo, error) {
	if previousBuildPos < 0 {
		return nil, errorutils.CheckErrorf("invalid input for previous build position. Input must be a non negative number")
	}

	// Create services manager to get build-info from Artifactory.
	sm, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return nil, err
	}

	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return nil, err
	}
	projectKey := buildConfiguration.GetProject()
	buildInfoParams := services.BuildInfoParams{BuildName: buildName, ProjectKey: projectKey}

	runs, found, err := sm.GetBuildRuns(buildInfoParams)
	if err != nil {
		return nil, err
	}
	// Return if build not found, or not enough build runs were returned to match the requested previous position.
	if !found || len(runs.BuildsNumbers)-1 < previousBuildPos {
		return &buildinfo.PublishedBuildInfo{}, nil
	}

	// Get matching build number. Build numbers are returned sorted, from latest to oldest.
	run := runs.BuildsNumbers[previousBuildPos]
	buildInfoParams.BuildNumber = strings.TrimPrefix(run.Uri, "/")

	publishedBuildInfo, found, err := sm.GetBuildInfo(buildInfoParams)
	if err != nil {
		return nil, err
	}
	// If build was deleted between requests.
	if !found {
		return &buildinfo.PublishedBuildInfo{}, nil
	}

	return publishedBuildInfo, nil
}

// Retrieves the build information of the first build that has a different VCS commit hash compared to the latest build.
// Iterates through previous builds in descending order until it finds a build with a different commit hash.
// Returns an empty build info struct if no such build is found or if there are no previous builds available.
func getPreviousBuildsCommit(serverDetails *utilsconfig.ServerDetails, buildConfiguration *build.BuildConfiguration) (*buildinfo.PublishedBuildInfo, error) {
	// Create services manager to get build-info from Artifactory.
	sm, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return nil, err
	}

	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return nil, err
	}
	projectKey := buildConfiguration.GetProject()
	buildInfoParams := services.BuildInfoParams{BuildName: buildName, ProjectKey: projectKey}

	runs, found, err := sm.GetBuildRuns(buildInfoParams)
	if err != nil {
		return nil, err
	}
	// Return if build not found, or not enough build runs were returned to match the requested previous position.
	if !found || len(runs.BuildsNumbers) == 0 {
		return &buildinfo.PublishedBuildInfo{}, nil
	}

	// Take the first log to get the reference for the first builds commit
	lastBuildInfoParams := services.BuildInfoParams{BuildName: buildName, ProjectKey: projectKey}
	lastBuildInfoParams.BuildNumber = strings.TrimPrefix(runs.BuildsNumbers[0].Uri, "/")
	lastPublishedBuildInfo, found, err := sm.GetBuildInfo(lastBuildInfoParams)
	if err != nil {
		return nil, err
	}
	// If build was deleted between requests.
	if !found {
		return &buildinfo.PublishedBuildInfo{}, nil
	}
	for _, run := range runs.BuildsNumbers {
		buildInfoParams.BuildNumber = strings.TrimPrefix(run.Uri, "/")

		publishedBuildInfo, found, err := sm.GetBuildInfo(buildInfoParams)
		if err != nil {
			return nil, err
		}
		// If build was deleted between requests.
		if !found {
			return &buildinfo.PublishedBuildInfo{}, nil
		}
		// If the commit hash is different from the last build, return the build info
		if publishedBuildInfo.BuildInfo.VcsList[0].Revision != lastPublishedBuildInfo.BuildInfo.VcsList[0].Revision {
			return publishedBuildInfo, nil
		}
	}
	return nil, errors.New("no previous builds commit has found")
}

func convertToUiLink(info *buildinfo.PublishedBuildInfo) (string, error) {
	datetime, err := ParseIsoTimestamp(info.BuildInfo.Started)
	if err != nil {
		return "", err
	}
	epochMillis := strconv.FormatInt(datetime.UnixNano()/1_000_000, 10)
	re := regexp.MustCompile(`(https://.+?)/artifactory/api/build/([^/]+)/([^?]+)(\?.+)?`)
	matches := re.FindStringSubmatch(info.Uri)

	if len(matches) < 4 {
		return "", errors.New("invalid API URL format")
	}

	baseUrl := matches[1]
	buildName := matches[2]
	buildNumber := matches[3]
	queryParams := ""
	if len(matches) == 5 {
		queryParams = matches[4] // Preserve query parameters if they exist
	}

	// Construct the UI-friendly URL
	uiUrl := strings.Join([]string{
		baseUrl,
		"ui/builds",
		buildName,
		buildNumber,
		epochMillis,
		"Evidence" + queryParams,
	}, "/")

	return uiUrl, nil
}

// Validates git is in path, and returns the VCS url by searching in the .git directory.
func validateGitAndGetVcsUrl(gitDetails *GitLogDetails) (string, error) {
	// Check that git exists in path.
	_, err := exec.LookPath("git")
	if err != nil {
		return "", errorutils.CheckError(err)
	}

	gitDetails.DotGitPath, err = GetDotGit(gitDetails.DotGitPath)
	if err != nil {
		return "", err
	}

	return getVcsUrl(gitDetails.DotGitPath)
}

func prepareGitLogCommand(gitDetails GitLogDetails, lastVcsRevision string) (logCmd *LogCmd, cleanupFunc func() error, err error) {
	// Get log with limit, starting from the latest commit.
	logCmd = &LogCmd{logLimit: gitDetails.LogLimit, lastVcsRevision: lastVcsRevision, prettyFormat: gitDetails.PrettyFormat}

	// Change working dir to where .git is.
	wd, err := os.Getwd()
	if errorutils.CheckError(err) != nil {
		return
	}
	cleanupFunc = func() error {
		return errors.Join(err, errorutils.CheckError(os.Chdir(wd)))
	}
	err = errorutils.CheckError(os.Chdir(gitDetails.DotGitPath))
	return
}

// Runs git log from lastVcsRevision to HEAD, using the provided format, and returns the output as is.
// Return RevisionRangeError if revision isn't found.
func getPlainGitLogFromLastVcsRevision(gitDetails GitLogDetails, lastVcsRevision string) (gitLog string, err error) {
	logCmd, cleanupFunc, err := prepareGitLogCommand(gitDetails, lastVcsRevision)
	defer func() {
		if cleanupFunc != nil {
			err = errors.Join(err, cleanupFunc())
		}
	}()

	stdOut, errorOut, _, err := gofrogcmd.RunCmdWithOutputParser(logCmd, false)
	if errorutils.CheckError(err) != nil {
		if strings.HasPrefix(strings.TrimSpace(errorOut), revisionRangeErrPrefix) {
			return "", getRevisionRangeError(lastVcsRevision)
		}
		return "", err
	}
	return stdOut, nil
}

// Creates a regexp handler to handle the event of revision missing in the git revision range.
func createErrRegExpHandler(lastVcsRevision string) (*gofrogcmd.CmdOutputPattern, error) {
	// Create regex pattern.
	invalidRangeExp, err := clientutils.GetRegExp(revisionRangeErrPrefix + ` [a-fA-F0-9]+\.\.`)
	if err != nil {
		return nil, err
	}

	// Create handler with exec function.
	errRegExp := gofrogcmd.CmdOutputPattern{
		RegExp: invalidRangeExp,
		ExecFunc: func(pattern *gofrogcmd.CmdOutputPattern) (string, error) {
			return "", getRevisionRangeError(lastVcsRevision)
		},
	}
	return &errRegExp, nil
}

func getRevisionRangeError(lastVcsRevision string) error {
	// Revision could not be found in the revision range, probably due to a squash / revert. Ignore and return.
	errMsg := "Revision: '" + lastVcsRevision + "' that was fetched from latest build info does not exist in the git revision range."
	return RevisionRangeError{ErrorMsg: errMsg}
}

// Gets vcs url from the .git directory.
func getVcsUrl(dotGitPath string) (string, error) {
	gitManager := clientutils.NewGitManager(dotGitPath)
	if err := gitManager.ReadConfig(); err != nil {
		return "", err
	}
	return gitManager.GetUrl(), nil
}

type LogCmd struct {
	logLimit        int
	lastVcsRevision string
	prettyFormat    string
}

func (logCmd *LogCmd) GetCmd() *exec.Cmd {
	var cmd []string
	cmd = append(cmd, "git")
	cmd = append(cmd, "log", "--pretty="+logCmd.prettyFormat, "-"+strconv.Itoa(logCmd.logLimit))
	if logCmd.lastVcsRevision != "" {
		cmd = append(cmd, logCmd.lastVcsRevision+"..")
	}
	return exec.Command(cmd[0], cmd[1:]...)
}

func (logCmd *LogCmd) GetEnv() map[string]string {
	return map[string]string{}
}

func (logCmd *LogCmd) GetStdWriter() io.WriteCloser {
	return nil
}

func (logCmd *LogCmd) GetErrWriter() io.WriteCloser {
	return nil
}

// Error to be thrown when revision could not be found in the git revision range.
type RevisionRangeError struct {
	ErrorMsg string
}

func (err RevisionRangeError) Error() string {
	return err.ErrorMsg
}
