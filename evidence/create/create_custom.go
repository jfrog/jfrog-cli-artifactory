package create

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"

	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/sigstore/sigstore-go/pkg/bundle"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/sigstore"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type createEvidenceCustom struct {
	createEvidenceBase
	subjectRepoPath       string
	subjectSha256         string
	sigstoreBundlePath    string
	autoSubjectResolution bool
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
		log.Info("Reading sigstore bundle from path:", c.sigstoreBundlePath)
		evidencePayload, err = c.processSigstoreBundle()
	} else {
		log.Info("Creating DSSE envelope for subject:", c.subjectRepoPath)
		evidencePayload, err = c.createDSSEEnvelope()
	}

	if err != nil {
		return err
	}

	err = c.validateSubject()
	if err != nil {
		return err
	}
	response, err := c.uploadEvidence(evidencePayload, c.subjectRepoPath)
	if err != nil {
		err = c.handleSubjectNotFound(err)
		return err
	}
	c.recordSummary(response)

	return nil
}

func (c *createEvidenceCustom) processSigstoreBundle() ([]byte, error) {
	sigstoreBundle, err := sigstore.ParseBundle(c.sigstoreBundlePath)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to read sigstore bundle: %s", err.Error())
	}

	if c.subjectRepoPath == "" {
		c.autoSubjectResolution = true
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
		return "", c.newSubjectError("Subject is not found in the sigstore bundle. Please ensure the bundle contains a valid subject.")
	} else {
		log.Info("Subject " + subject + " is resolved from sigstore bundle.")
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

func (c *createEvidenceCustom) validateSubject() error {
	// Pattern: must have at least one slash with non-empty sections
	if matched, _ := regexp.MatchString(`^[^/]+(/[^/]+)+$`, c.subjectRepoPath); !matched {
		return c.newSubjectError("Subject '" + c.subjectRepoPath + "' is invalid. Subject must be in format: <repo>/<path>/<name> or <repo>/<name>")
	}
	return nil
}

func (c *createEvidenceCustom) handleSubjectNotFound(err error) error {
	errStr := err.Error()
	if strings.Contains(errStr, "404 Not Found") {
		log.Debug("Server response error:", err.Error())
		return c.newSubjectError("Subject '" + c.subjectRepoPath + "' is not found. Please ensure the subject exists.")
	}
	return err
}

// newSubjectError creates an error with ExitCodeFailNoOp (2) for subject-related failures
// When auto subject resolution is enabled, this allows pipeline calls with gh attestation
// sigstore bundle generation to skip command execution without breaking a pipeline
func (c *createEvidenceCustom) newSubjectError(message string) error {
	if c.autoSubjectResolution {
		return coreutils.CliError{
			ExitCode: coreutils.ExitCodeFailNoOp,
			ErrorMsg: message,
		}
	}
	return errorutils.CheckErrorf("%s", message)
}

func (c *createEvidenceCustom) recordSummary(response *model.CreateResponse) {
	commandSummary := commandsummary.EvidenceSummaryData{
		Subject:       c.subjectRepoPath,
		SubjectSha256: c.subjectSha256,
		PredicateType: response.PredicateType,
		PredicateSlug: response.PredicateSlug,
		Verified:      response.Verified,
		DisplayName:   c.subjectRepoPath,
		SubjectType:   commandsummary.SubjectTypeArtifact,
	}
	err := c.recordEvidenceSummary(commandSummary)
	if err != nil {
		log.Warn("Failed to record evidence summary:", err.Error())
	}
}
