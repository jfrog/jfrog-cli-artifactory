package agentcommon

import (
	"strings"

	"github.com/jfrog/jfrog-cli-evidence/evidence/create"

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

// CreateEvidence attaches a signed publish-attestation to an artifact using jfrog-cli-evidence programmatically.
func CreateEvidence(serverDetails *config.ServerDetails, opts CreateEvidenceOpts) error {
	sd := *serverDetails
	ensureServiceUrls(&sd)
	cmd := create.NewCreateEvidenceCustom(
		&sd,
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

// ensureServiceUrls derives the platform URL from ArtifactoryUrl and populates
// service-specific URLs that the evidence library requires. It mutates sd in place.
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
