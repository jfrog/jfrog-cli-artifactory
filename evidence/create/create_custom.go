package create

import (
	"encoding/json"
	clientLog "github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"regexp"
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
		clientLog.Info("Reading sigstore bundle from path:", c.sigstoreBundlePath)
		evidencePayload, err = c.processSigstoreBundle()
	} else {
		clientLog.Info("Creating DSSE envelope for subject:", c.subjectRepoPath)
		evidencePayload, err = c.createDSSEEnvelope()
	}

	if err != nil {
		return err
	}

	err = validateSubject(c.subjectRepoPath)
	if err != nil {
		return err
	}
	err = c.uploadEvidence(evidencePayload, c.subjectRepoPath)
	if err != nil {
		err = handleSubjectNotFound(err, c.subjectRepoPath)
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
	subject, err := sigstore.ExtractSubjectFromBundle(bundle)
	if err != nil {
		return "", err
	}

	if subject == "" {
		return "", errorutils.CheckErrorf("Subject is not found in the sigstore bundle. Please ensure the bundle contains a valid subject.")
	} else {
		clientLog.Info("Subject " + subject + " is resolved from sigstore bundle.")
	}

	return subject, nil
}

func (c *createEvidenceCustom) createDSSEEnvelope() ([]byte, error) {
	envelope, err := c.createEnvelope(c.subjectRepoPath, c.subjectSha256)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}

func validateSubject(subject string) error {
	// Pattern: must have at least one slash with non-empty sections
	if matched, _ := regexp.MatchString(`^[^/]+(/[^/]+)+$`, subject); !matched {
		return errorutils.CheckErrorf("Subject '%s' is invalid. Subject must be in format: <repo>/<path>/<name> or <repo>/<name>", subject)
	}
	return nil
}

func handleSubjectNotFound(err error, subject string) error {
	errStr := err.Error()
	if strings.Contains(errStr, "404 Not Found") {
		clientLog.Debug("Server response error:", err.Error())
		return errorutils.CheckErrorf("Subject '%s' is not found. Please ensure the subject exists.", subject)
	}
	return err
}
