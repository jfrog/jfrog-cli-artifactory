package create

import (
	"os"
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

// processSigstoreBundle reads and processes a Sigstore bundle, returning the envelope and subject
func (c *createEvidenceCustom) processSigstoreBundle() ([]byte, error) {
	// Read the Sigstore bundle file
	bundle, err := os.ReadFile(c.sigstoreBundlePath)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to read sigstore bundle: %s", err.Error())
	}

	// Only extract subject from bundle if current subject is empty
	if c.subjectRepoPath == "" {
		extractedSubject, err := c.extractSubjectFromBundle()
		if err != nil {
			return nil, err
		}
		c.subjectRepoPath = extractedSubject
	}

	return bundle, nil
}

func (c *createEvidenceCustom) extractSubjectFromBundle() (string, error) {
	// Parse the bundle first
	bundle, err := sigstore.ParseBundle(c.sigstoreBundlePath)
	if err != nil {
		return "", err
	}

	// Extract subject from the parsed bundle
	repoPath, err := sigstore.ExtractSubjectFromBundle(bundle)
	if err != nil {
		return "", err
	}

	return repoPath, nil
}

// createDSSEEnvelope creates a DSSE envelope from the provided predicate and subject information
func (c *createEvidenceCustom) createDSSEEnvelope() ([]byte, error) {
	envelope, err := c.createEnvelope(c.subjectRepoPath, c.subjectSha256)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}
