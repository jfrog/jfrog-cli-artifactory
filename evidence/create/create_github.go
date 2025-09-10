package create

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/jfrog/froggit-go/vcsclient"
	"github.com/jfrog/froggit-go/vcsutils"
	artifactoryUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type FlagType string

const (
	FlagTypeCommitterReviewer FlagType = "gh-commiter"
	FlagTypeOther             FlagType = "other"
)

const ghDefaultPredicateType = "https://jfrog.com/evidence/gh-commiter/v1"

const gitFormat = `"{\"commit\":\"%H\",\"abbreviated_commit\":\"%h\",\"tree\":\"%T\",\"abbreviated_tree\":\"%t\",\"parent\":\"%P\",\"abbreviated_parent\":\"%p\",\"subject\":\"%s\",\"sanitized_subject_line\":\"%f\",\"author\":{\"name\":\"%an\",\"email\":\"%ae\",\"date\":\"%ad\"},\"commiter\":{\"name\":\"%cn\",\"email\":\"%ce\",\"date\":\"%cd\"}}"`

// Package-level function variables for testing
var (
	getPlainGitLogFromPreviousBuild = artifactoryUtils.GetPlainGitLogFromPreviousBuild
	getLastBuildLink                = artifactoryUtils.GetLastBuildLink
	newVcsClient                    = func(token string) (pullRequestClient, error) {
		return vcsclient.NewClientBuilder(vcsutils.GitHub).Token(token).Build()
	}
)

// pullRequestClient is a minimal interface for what we actually use from vcsclient
type pullRequestClient interface {
	ListPullRequestsAssociatedWithCommit(ctx context.Context, owner, repository, commit string) ([]vcsclient.PullRequestInfo, error)
	ListPullRequestReviews(ctx context.Context, owner, repository string, prID int) ([]vcsclient.PullRequestReviewDetails, error)
}

type createGitHubEvidence struct {
	createEvidenceBase
	project     string
	buildName   string
	buildNumber string
}

func NewCreateGithub(serverDetails *config.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, buildName, buildNumber, typeFlag string) evidence.Command {
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
		project:     project,
		buildName:   buildName,
		buildNumber: buildNumber,
	}
}

func getFlagType(typeFlag string) FlagType {
	var flagType FlagType
	if typeFlag == "gh-commiter" {
		flagType = FlagTypeCommitterReviewer
	} else {
		flagType = FlagTypeOther
	}
	return flagType
}

func (c *createGitHubEvidence) CommandName() string {
	return "create-github-evidence"
}

func (c *createGitHubEvidence) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createGitHubEvidence) Run() error {
	if !utils.IsRunningUnderGitHubAction() {
		return errors.New("this command is intended to be run under GitHub Actions")
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
	subject, sha256, err := c.buildBuildInfoSubjectPath(artifactoryClient)
	if err != nil {
		return err
	}
	envelope, err := c.createEnvelopeWithPredicateAndPredicateType(subject,
		sha256, ghDefaultPredicateType, evidencePredicate)
	if err != nil {
		return err
	}
	_, err = c.uploadEvidence(envelope, subject)
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

	repoKey := utils.BuildBuildInfoRepoKey(c.project)
	buildInfoPath := buildBuildInfoPath(repoKey, c.buildName, c.buildNumber, timestamp)
	buildInfoChecksum, err := getBuildInfoPathChecksum(buildInfoPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}
	return buildInfoPath, buildInfoChecksum, nil
}

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
	link, err := getLastBuildLink(c.serverDetails, buildConfiguration)
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
		return nil, errors.New("flag type must be gh-commiter")
	}

	createBuildConfiguration := c.createBuildConfiguration()
	gitDetails := artifactoryUtils.GitLogDetails{LogLimit: 100, PrettyFormat: gitFormat}
	committerEvidence, err := c.getGitCommitInfo(c.serverDetails, createBuildConfiguration, gitDetails)
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

func (c *createGitHubEvidence) getGitCommitInfo(serverDetails *config.ServerDetails, createBuildConfiguration *build.BuildConfiguration, gitDetails artifactoryUtils.GitLogDetails) ([]byte, error) {
	owner, repository, err := gitHubRepositoryDetails()
	if err != nil {
		return nil, err
	}

	entries, err := c.getGitCommitEntries(serverDetails, createBuildConfiguration, gitDetails)
	if err != nil {
		return nil, err
	}

	ghToken, err := utils.GetEnvVariable("JF_GIT_TOKEN")
	if err != nil {
		return nil, err
	}

	// Create VCS client
	client, err := newVcsClient(ghToken)
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

func (c *createGitHubEvidence) getGitCommitEntries(serverDetails *config.ServerDetails, createBuildConfiguration *build.BuildConfiguration, gitDetails artifactoryUtils.GitLogDetails) ([]model.GitLogEntry, error) {
	fullLog, err := getPlainGitLogFromPreviousBuild(serverDetails, createBuildConfiguration, gitDetails)
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
