package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jfrog/froggit-go/vcsclient"
	"github.com/jfrog/froggit-go/vcsutils"
	artifactoryUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/http/httpclient"
	rtutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type FlagType string

const (
	FlagTypeCommitterReviewer FlagType = "gh-commiter"
	FlagTypeOther             FlagType = "other"
)
const releaseBundleInternalApi = "api/v2/release_bundle/internal/graph/"
const releaseBundleApi = "api/v2/release_bundle/records/"

const ghDefaultPredicateType = "https://jfrog.com/evidence/git-committer-reviewer/v1"

const gitFormat = `format:'{"commit":"%H","abbreviated_commit":"%h","tree":"%T","abbreviated_tree":"%t","parent":"%P","abbreviated_parent":"%p","subject":"%s","sanitized_subject_line":"%f","author":{"name":"%aN","email":"%aE","date":"%aD"},"commiter":{"name":"%cN","email":"%cE","date":"%cD"}}'`

type createGitHubEvidence struct {
	createEvidenceBase
	project              string
	buildName            string
	buildNumber          string
	releaseBundle        string
	releaseBundleVersion string
}

func NewCreateGithub(serverDetails *coreConfig.ServerDetails,
	predicateFilePath, predicateType, markdownFilePath, key, keyId, project, buildName, buildNumber, typeFlag, rbName, rbVersion string) Command {
	flagType := getFlagType(typeFlag)
	return &createGitHubEvidence{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
			flagType:          flagType,
		},
		project:              project,
		buildName:            buildName,
		buildNumber:          buildNumber,
		releaseBundle:        rbName,
		releaseBundleVersion: rbVersion,
	}
}

func getFlagType(typeFlag string) FlagType {
	flagTypes := map[string]FlagType{
		"gh-commiter": FlagTypeCommitterReviewer,
	}

	if flag, exists := flagTypes[typeFlag]; exists {
		return flag
	}
	return FlagTypeOther
}

func (c *createGitHubEvidence) CommandName() string {
	return "create-github-evidence"
}

func (c *createGitHubEvidence) ServerDetails() (*coreConfig.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createGitHubEvidence) Run() error {
	if !isRunningUnderGitHubAction() {
		return errors.New("this command is intended to be run under GitHub Actions")
	}
	if c.buildName == "" && c.releaseBundle == "" {
		return errors.New("build name or release bundle name is required")
	}

	if c.releaseBundle != "" {
		err := c.getBuildFromReleaseBundle()
		if err != nil {
			return err
		}
	}

	evidencePredicate, err := c.committerReviewerEvidence()
	if err != nil {
		return err
	}

	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return err
	}

	var subject, sha256 string
	if c.releaseBundle != "" && c.releaseBundleVersion != "" {
		subject, sha256, err = c.buildReleaseBundleSubjectPath(artifactoryClient)
		if err != nil {
			return err
		}
	} else {
		subject, sha256, err = c.buildBuildInfoSubjectPath(artifactoryClient)
		if err != nil {
			return err
		}
	}
	envelope, err := c.createEnvelopeWithPredicateAndPredicateType(subject,
		sha256, ghDefaultPredicateType, evidencePredicate)
	if err != nil {
		return err
	}
	err = c.uploadEvidence(envelope, subject)
	if err != nil {
		return err
	}

	err = c.recordEvidenceSummaryData(evidencePredicate, subject, sha256)
	if err != nil {
		return err
	}

	return nil
}

func (c *createGitHubEvidence) buildBuildInfoSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, string, error) {
	timestamp, err := getBuildLatestTimestamp(c.buildName, c.buildNumber, c.project, artifactoryClient)
	if err != nil {
		return "", "", err
	}

	repoKey := buildBuildInfoRepoKey(c.project)
	buildInfoPath := buildBuildInfoPath(repoKey, c.buildName, c.buildNumber, timestamp)
	buildInfoChecksum, err := getBuildInfoPathChecksum(buildInfoPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}
	return buildInfoPath, buildInfoChecksum, nil
}

func (c *createGitHubEvidence) buildReleaseBundleSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, string, error) {
	repoKey := buildRepoKey(c.project)
	manifestPath := buildManifestPath(repoKey, c.releaseBundle, c.releaseBundleVersion)

	manifestChecksum, err := c.getFileChecksum(manifestPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}

	return manifestPath, manifestChecksum, nil
}

func isRunningUnderGitHubAction() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// This function will print the markdown to GH
func (c *createGitHubEvidence) recordEvidenceSummaryData(evidence []byte, subject string, subjectSha256 string) error {
	commandSummary, err := commandsummary.NewBuildInfoSummary()
	if err != nil {
		return err
	}

	gitLogModel, err := marshalEvidenceToGitLogEntryView(evidence)
	if err != nil {
		return err
	}
	link, err := c.getLastBuildLink()
	if err != nil {
		return err
	}
	gitLogModel.Link = link
	gitLogModel.Artifact.Path = subject
	gitLogModel.Artifact.Sha256 = subjectSha256
	gitLogModel.Artifact.Name = c.buildName

	err = commandSummary.RecordWithIndex(gitLogModel, commandsummary.Evidence)
	if err != nil {
		return err
	}
	return nil
}

func (c *createGitHubEvidence) getLastBuildLink() (string, error) {
	buildConfiguration := new(build.BuildConfiguration)
	buildConfiguration.SetBuildName(c.buildName).SetBuildNumber(c.buildName).SetProject(c.project)
	link, err := artifactoryUtils.GetLastBuildLink(c.serverDetails, buildConfiguration)
	if err != nil {
		return "", err
	}
	return link, nil
}

func marshalEvidenceToGitLogEntryView(evidence []byte) (*model.GitLogEntryView, error) {
	var gitLogEntryView model.GitLogEntryView
	err := json.Unmarshal(evidence, &gitLogEntryView.Data)
	if err != nil {
		return nil, err
	}
	return &gitLogEntryView, nil
}

func (c *createGitHubEvidence) committerReviewerEvidence() ([]byte, error) {
	if c.createEvidenceBase.flagType != FlagTypeCommitterReviewer {
		return nil, errors.New("flag type is not supported")
	}

	createBuildConfiguration := c.createBuildConfiguration()
	gitDetails := artifactoryUtils.GitLogDetails{LogLimit: 100, PrettyFormat: gitFormat}
	committerEvidence, err := getGitCommitInfo(c.serverDetails, createBuildConfiguration, gitDetails)
	if err != nil {
		return nil, err
	}
	return committerEvidence, nil
}

func (c *createGitHubEvidence) createBuildConfiguration() *build.BuildConfiguration {
	buildConfiguration := new(build.BuildConfiguration)
	buildConfiguration.SetBuildName(c.buildName).SetBuildNumber(c.buildNumber).SetProject(c.project)
	return buildConfiguration
}

func (c *createGitHubEvidence) getBuildFromReleaseBundle() error {
	releaseBundleResponse, err := c.getPreviousReleaseBundle()
	if err != nil {
		return err
	}
	if len(releaseBundleResponse.ReleaseBundles) == 0 {
		return errors.New("no release bundles found")
	}
	if len(releaseBundleResponse.ReleaseBundles) > 1 {
		// Get the previous release bundle
		c.releaseBundleVersion = releaseBundleResponse.ReleaseBundles[1].ReleaseBundleVersion
	} else {
		c.releaseBundleVersion = releaseBundleResponse.ReleaseBundles[0].ReleaseBundleVersion
	}

	rbv2Graph, err := c.getReleaseBundleGraph()
	if err != nil {
		return err
	}
	for _, node := range rbv2Graph.Root.Nodes {
		if node.Type == "buildinfo" {
			c.buildName = node.Name
			c.buildNumber = node.Version
			break
		}
	}
	// Get the current release bundle to put the evidence on
	c.releaseBundleVersion = releaseBundleResponse.ReleaseBundles[0].ReleaseBundleVersion
	return nil
}

func (c *createGitHubEvidence) getReleaseBundleGraph() (*model.GraphResponse, error) {
	authConfig, err := c.serverDetails.CreateArtAuthConfig()
	if err != nil {
		return nil, err
	}

	artifactoryApiUrl, err := rtutils.BuildUrl(c.serverDetails.GetLifecycleUrl(), releaseBundleInternalApi+c.releaseBundle+"/"+c.releaseBundleVersion, make(map[string]string))
	if err != nil {
		return nil, err
	}

	artHttpDetails := authConfig.CreateHttpClientDetails()
	client, err := httpclient.ClientBuilder().Build()
	if err != nil {
		return nil, err
	}
	resp, body, _, err := client.SendGet(artifactoryApiUrl, true, artHttpDetails, "")
	if err != nil {
		return nil, err
	}
	if err = errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); err != nil {
		return nil, err
	}
	graphResponse := &model.GraphResponse{}
	if err = json.Unmarshal(body, &graphResponse); err != nil {
		return nil, errorutils.CheckError(err)
	}
	return graphResponse, nil
}

func (c *createGitHubEvidence) getPreviousReleaseBundle() (*model.ReleaseBundlesResponse, error) {
	authConfig, err := c.serverDetails.CreateArtAuthConfig()
	if err != nil {
		return nil, err
	}

	queryParams := map[string]string{
		"project": c.project,
	}
	artifactoryApiUrl, err := rtutils.BuildUrl(c.serverDetails.GetLifecycleUrl(), releaseBundleApi+c.releaseBundle, queryParams)
	if err != nil {
		return nil, err
	}

	artHttpDetails := authConfig.CreateHttpClientDetails()
	client, err := httpclient.ClientBuilder().Build()
	if err != nil {
		return nil, err
	}
	resp, body, _, err := client.SendGet(artifactoryApiUrl, true, artHttpDetails, "")
	if err != nil {
		return nil, err
	}
	if err = errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); err != nil {
		return nil, err
	}

	response := &model.ReleaseBundlesResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, errorutils.CheckError(err)
	}
	return response, nil
}

type PackageVersionResponseContent struct {
	Version string `json:"Version,omitempty"`
}

func getGitCommitInfo(serverDetails *coreConfig.ServerDetails, createBuildConfiguration *build.BuildConfiguration, gitDetails artifactoryUtils.GitLogDetails) ([]byte, error) {
	owner, repository, err := gitHubRepositoryDetails()
	log.Info(fmt.Sprintf("owner: %s repository: %s", owner, repository))
	if err != nil {
		return nil, err
	}

	entries, err := getGitCommitEntries(serverDetails, createBuildConfiguration, gitDetails)
	if err != nil {
		return nil, err
	}

	ghToken, err := utils.GetEnvVariable("JF_GIT_TOKEN")
	if err != nil {
		return nil, err
	}

	client, err := vcsclient.NewClientBuilder(vcsutils.GitHub).Token(ghToken).Build()
	if err != nil {
		return nil, err
	}

	for i := range entries {
		prMetadata, err := client.ListPullRequestsAssociatedWithCommit(context.Background(), owner, repository, entries[i].Commit)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to get PR metadata for commit: %s, error: %v", entries[i].Commit, err))
			continue
		}

		if len(prMetadata) == 0 {
			log.Info(fmt.Sprintf("No PR metadata found for commit: %s", entries[i].Commit))
			entries[i].PRreviewer = []vcsclient.PullRequestReviewDetails{}
			continue
		}

		prReviewer, err := client.ListPullRequestReviews(context.Background(), owner, repository, int(prMetadata[0].ID))
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to get PR reviews for PR ID %d: %v", prMetadata[0].ID, err))
			entries[i].PRreviewer = []vcsclient.PullRequestReviewDetails{}
			continue
		}

		entries[i].PRreviewer = make([]vcsclient.PullRequestReviewDetails, len(prReviewer))
		copy(entries[i].PRreviewer, prReviewer)
	}

	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, err
	}
	return out, nil
}

func getGitCommitEntries(serverDetails *coreConfig.ServerDetails, createBuildConfiguration *build.BuildConfiguration, gitDetails artifactoryUtils.GitLogDetails) ([]model.GitLogEntry, error) {
	fullLog, err := artifactoryUtils.GetPlainGitLogFromPreviousBuild(serverDetails, createBuildConfiguration, gitDetails)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`'(\{.*?\})'`)
	matches := re.FindAllStringSubmatch(fullLog, -1)

	var entries []model.GitLogEntry
	for _, m := range matches {
		jsonText := m[1] // The captured group is the JSON object
		var entry model.GitLogEntry
		if err := json.Unmarshal([]byte(jsonText), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func gitHubRepositoryDetails() (string, string, error) {
	githubRepo := os.Getenv("GITHUB_REPOSITORY") // Format: "owner/repository"
	if githubRepo == "" {
		return "", "", fmt.Errorf("GITHUB_REPOSITORY environment variable is not set")
	}

	parts := strings.Split(githubRepo, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid GITHUB_REPOSITORY format: %s", githubRepo)
	}
	owner, repository := parts[0], parts[1]
	return owner, repository, nil
}
