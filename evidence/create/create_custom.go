package create

import (
	"encoding/json"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/sigstore"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type createEvidenceCustom struct {
	createEvidenceBase
	subjectRepoPath    string
	subjectSha256      string
	sigstoreBundlePath string
}

func NewCreateEvidenceCustom(serverDetails *config.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, subjectRepoPath,
	subjectSha256, sigstoreBundlePath, providerId string) evidence.Command {
	return &createEvidenceCustom{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			providerId:        providerId,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
		},
		subjectRepoPath:    subjectRepoPath,
		subjectSha256:      subjectSha256,
		sigstoreBundlePath: sigstoreBundlePath,
	}
}

func (c *createEvidenceCustom) CommandName() string {
	return "create-custom-evidence"
}

func (c *createEvidenceCustom) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidenceCustom) Run() error {
	var evidencePayload []byte
	var err error

	if c.sigstoreBundlePath != "" {
		evidencePayload, err = c.processSigstoreBundle()
	} else {
		evidencePayload, err = c.createDSSEEnvelope()
	}

	if err != nil {
		return err
	}

	err = c.uploadEvidence(evidencePayload, c.subjectRepoPath)
	if err != nil {
		if strings.Contains(err.Error(), "evidence selector must be of {repository}/{path}/{name}") {
			return errorutils.CheckErrorf("Invalid subject format: '%s'. Subject must be in format: <repo>/<path>/<name> or <repo>/<name>", c.subjectRepoPath)
		}
		return err
	}

	return nil
}

func (c *createEvidenceCustom) processSigstoreBundle() ([]byte, error) {
	sigstoreBundle, err := sigstore.ParseBundle(c.sigstoreBundlePath)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to read sigstore bundle: %s", err.Error())
	}

	if c.subjectRepoPath == "" {

		extractedSubject, err := c.extractSubjectFromBundle(sigstoreBundle)
		if err != nil {
			return nil, err
		}
		c.subjectRepoPath = extractedSubject
	}

	return json.Marshal(sigstoreBundle)
}

func (c *createEvidenceCustom) extractSubjectFromBundle(bundle *bundle.Bundle) (string, error) {
	repoPath, err := sigstore.ExtractSubjectFromBundle(bundle)
	if err != nil {
		return "", err
	}

	return repoPath, nil
}

func (c *createEvidenceCustom) createDSSEEnvelope() ([]byte, error) {
	envelope, err := c.createEnvelope(c.subjectRepoPath, c.subjectSha256)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}
