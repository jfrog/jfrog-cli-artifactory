package common

import (
	"strings"

	"github.com/jfrog/jfrog-cli-evidence/evidence/create"
	"github.com/jfrog/jfrog-cli-evidence/evidence/verify"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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

type VerifyEvidenceOpts struct {
	SubjectRepoPath string
}

// CreateEvidence attaches a signed publish-attestation to an artifact using jfrog-cli-evidence programmatically.
func CreateEvidence(serverDetails *config.ServerDetails, opts CreateEvidenceOpts) error {
	ensureServiceUrls(serverDetails)
	cmd := create.NewCreateEvidenceCustom(
		serverDetails,
		opts.PredicatePath,
		opts.PredicateType,
		opts.MarkdownPath,
		opts.KeyPath,
		opts.KeyAlias,
		opts.SubjectRepoPath,
		opts.SubjectSHA256,
		"", "", "",
	)
	return cmd.Run()
}

// VerifyEvidence verifies publish-attestation evidence on an artifact using Artifactory keys.
func VerifyEvidence(serverDetails *config.ServerDetails, opts VerifyEvidenceOpts) error {
	ensureServiceUrls(serverDetails)
	cmd := verify.NewVerifyEvidenceCustom(
		serverDetails,
		opts.SubjectRepoPath,
		"plaintext",
		nil,
		true,
	)
	return cmd.Run()
}

// ensureServiceUrls derives the platform URL from ArtifactoryUrl and populates
// service-specific URLs that the evidence library requires.
func ensureServiceUrls(sd *config.ServerDetails) {
	if sd.Url != "" {
		platformBase := clientutils.AddTrailingSlashIfNeeded(sd.Url)
		if sd.OnemodelUrl == "" {
			sd.OnemodelUrl = platformBase + "onemodel/"
		}
		if sd.EvidenceUrl == "" {
			sd.EvidenceUrl = platformBase + "evidence/"
		}
		return
	}

	if sd.ArtifactoryUrl == "" {
		return
	}

	platformBase := sd.ArtifactoryUrl
	platformBase = strings.TrimRight(platformBase, "/")
	platformBase = strings.TrimSuffix(platformBase, "/artifactory")
	platformBase = clientutils.AddTrailingSlashIfNeeded(platformBase)

	sd.Url = platformBase
	if sd.OnemodelUrl == "" {
		sd.OnemodelUrl = platformBase + "onemodel/"
	}
	if sd.EvidenceUrl == "" {
		sd.EvidenceUrl = platformBase + "evidence/"
	}
}
