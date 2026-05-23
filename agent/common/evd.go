package common

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-evidence/evidence/create"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
)

type CreateEvidenceOpts struct {
	SubjectRepoPath string
	SubjectSHA256   string
	PredicatePath   string
	PredicateType   string
	MarkdownPath    string
	KeyPath         string
	KeyAlias        string
}

// CreateEvidence attaches a signed publish-attestation to an artifact using jfrog-cli-evidence programmatically.
func CreateEvidence(serverDetails *config.ServerDetails, opts CreateEvidenceOpts) error {
	localServerDetails := *serverDetails
	ensureServiceUrls(&localServerDetails)
	cmd := create.NewCreateEvidenceCustom(
		&localServerDetails,
		opts.PredicatePath,
		opts.PredicateType,
		opts.MarkdownPath,
		opts.KeyPath,
		opts.KeyAlias,
		opts.SubjectRepoPath,
		opts.SubjectSHA256,
		"", "", "",
		"", "", "",
	)
	return cmd.Run()
}

// ensureServiceUrls populates service-specific URLs that the evidence library requires.
// Platform URL comes from config.ServerDetails (Url / ArtifactoryUrl via normalizeArtifactoryUrl).
func ensureServiceUrls(localServerDetails *config.ServerDetails) {
	normalizeArtifactoryUrl(localServerDetails)
	platformBase := clientutils.AddTrailingSlashIfNeeded(localServerDetails.GetUrl())
	if platformBase == "" {
		return
	}
	if localServerDetails.GetOnemodelUrl() == "" {
		localServerDetails.OnemodelUrl = platformBase + "onemodel/"
	}
	if localServerDetails.GetEvidenceUrl() == "" {
		localServerDetails.EvidenceUrl = platformBase + "evidence/"
	}
}
