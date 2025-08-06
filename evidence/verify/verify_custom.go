package verify

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const aqlCustomQueryTemplate = "items.find({\"repo\": \"%s\",%s\"name\": \"%s\"}).include(\"repo\", \"path\", \"name\", \"sha256\")"
const optionalPathTemplate = "\"path\": \"%s\","

// verifyEvidenceCustom verifies evidence for a custom subject path.
type verifyEvidenceCustom struct {
	verifyEvidenceBase
	subjectRepoPath string
}

// NewVerifyEvidenceCustom creates a new command for verifying evidence for a custom subject path.
func NewVerifyEvidenceCustom(serverDetails *config.ServerDetails, subjectRepoPath, format string, keys []string, useArtifactoryKeys bool) evidence.Command {
	return &verifyEvidenceCustom{
		verifyEvidenceBase: verifyEvidenceBase{
			serverDetails:      serverDetails,
			format:             format,
			keys:               keys,
			useArtifactoryKeys: useArtifactoryKeys,
		},
		subjectRepoPath: subjectRepoPath,
	}
}

// Run executes the custom evidence verification command.
func (v *verifyEvidenceCustom) Run() error {
	repo, path, name, err := extractSubjectRepoPathName(v)
	if err != nil {
		return err
	}
	client, err := v.createArtifactoryClient()
	if err != nil {
		return fmt.Errorf("failed to create Artifactory client: %w", err)
	}
	query := prepareAqlQuery(path, repo, name)
	result, err := utils.ExecuteAqlQuery(query, client)
	if err != nil {
		return fmt.Errorf("failed to execute AQL query: %w", err)
	}
	if len(result.Results) == 0 {
		return fmt.Errorf("no subject found for %s", v.subjectRepoPath)
	}
	metadata, err := v.queryEvidenceMetadata(repo, path, name)
	if err != nil {
		return err
	}
	subjectSha256 := result.Results[0].Sha256
	return v.verifyEvidence(client, metadata, subjectSha256, v.subjectRepoPath)
}

func extractSubjectRepoPathName(v *verifyEvidenceCustom) (string, string, string, error) {
	split := strings.Split(v.subjectRepoPath, "/")
	if len(split) < 2 {
		return "", "", "", fmt.Errorf("invalid subject repository path: %s. Expected format: <repo>/<path>/<name> or <repo>/<name>", v.subjectRepoPath)
	}
	repo := split[0]
	name := split[len(split)-1]
	path := strings.Join(split[1:len(split)-1], "/")
	return repo, path, name, nil
}

// ServerDetails returns the server details for the command.
func (v *verifyEvidenceCustom) ServerDetails() (*config.ServerDetails, error) {
	return v.serverDetails, nil
}

// CommandName returns the name of the command.
func (v *verifyEvidenceCustom) CommandName() string {
	return "verify-evidence-custom"
}

func prepareAqlQuery(path string, repo string, name string) string {
	if path != "" {
		path = fmt.Sprintf(optionalPathTemplate, path)
	}
	query := fmt.Sprintf(aqlCustomQueryTemplate, repo, path, name)
	return query
}
