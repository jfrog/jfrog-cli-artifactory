package create

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/create/resolvers"
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

const subjectsLimit = 10

type createEvidenceCustom struct {
	createEvidenceBase
	subjectRepoPaths      []string
	subjectSha256         string
	sigstoreBundlePath    string
	autoSubjectResolution bool
}

func NewCreateEvidenceCustom(serverDetails *config.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, subjectRepoPath,
	subjectSha256, sigstoreBundlePath, providerId string) evidence.Command {
	var subjectRepoPathSlice []string
	if subjectRepoPath != "" {
		subjectRepoPathSlice = []string{subjectRepoPath}
	} else {
		subjectRepoPathSlice = []string{}
	}
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
		subjectRepoPaths:   subjectRepoPathSlice,
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
		log.Info("Creating DSSE envelope for subject:", c.subjectRepoPaths)
		evidencePayload, err = c.createDSSEEnvelope()
	}

	if err != nil {
		return err
	}

	var errors []error
	var successfulSubjects []string
	var failedSubjects []string

	if len(c.subjectRepoPaths) > subjectsLimit {
		return fmt.Errorf("too many subjects resolved (%d). Maximum allowed is %d", len(c.subjectRepoPaths), subjectsLimit)
	}

	for _, subjectRepoPath := range c.subjectRepoPaths {
		if err := c.validateSubject(subjectRepoPath); err != nil {
			log.Error("Subject validation failed for", subjectRepoPath, ":", err.Error())
			errors = append(errors, fmt.Errorf("validation failed for subject '%s': %w", subjectRepoPath, err))
			failedSubjects = append(failedSubjects, subjectRepoPath)
			continue
		}

		response, err := c.uploadEvidence(evidencePayload, subjectRepoPath)
		if err != nil {
			handledErr := c.handleSubjectNotFound(subjectRepoPath, err)
			log.Error("Evidence upload failed for", subjectRepoPath, ":", handledErr.Error())
			errors = append(errors, fmt.Errorf("upload failed for subject '%s': %w", subjectRepoPath, handledErr))
			failedSubjects = append(failedSubjects, subjectRepoPath)
			continue
		}

		c.recordSummary(subjectRepoPath, response)
		successfulSubjects = append(successfulSubjects, subjectRepoPath)
		log.Info("Successfully processed subject:", subjectRepoPath)
	}

	// Report results
	if len(successfulSubjects) > 0 {
		log.Info("Successfully processed", len(successfulSubjects), "subjects:", strings.Join(successfulSubjects, ", "))
	}

	if len(failedSubjects) > 0 {
		log.Error("Failed to process", len(failedSubjects), "subjects:", strings.Join(failedSubjects, ", "))
	}

	// Determine final error behavior based on configuration and results
	return c.determineFinalError(errors, successfulSubjects, failedSubjects)
}

func (c *createEvidenceCustom) processSigstoreBundle() ([]byte, error) {
	sigstoreBundle, err := sigstore.ParseBundle(c.sigstoreBundlePath)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to read sigstore bundle: %s", err.Error())
	}

	if len(c.subjectRepoPaths) == 0 {
		c.autoSubjectResolution = true
		extractedSubject, err := c.extractSubjectFromBundle(sigstoreBundle)
		if err != nil {
			return nil, err
		}
		c.subjectRepoPaths = extractedSubject
	}

	return json.Marshal(sigstoreBundle)
}

func (c *createEvidenceCustom) extractSubjectFromBundle(bundle *bundle.Bundle) ([]string, error) {
	subject, sha256, err := sigstore.ExtractSubjectFromBundle(bundle)
	if err != nil {
		return nil, fmt.Errorf("failed to extract subject from bundle: %w", err)
	}

	if subject == "" {
		return nil, c.newSubjectError("Subject is not found in the sigstore bundle. Please ensure the bundle contains a valid subject.")
	}

	client, err := c.createArtifactoryClient()
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to create Artifactory client: %s", err.Error())
	}

	log.Info("Resolving subject from bundle:", subject, "with checksum:", sha256)
	subjects, err := resolvers.ResolveSubject(subject, sha256, client)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to resolve subject '%s' with checksum '%s': %s", subject, sha256, err.Error())
	}

	if len(subjects) == 0 {
		return nil, c.newSubjectError(fmt.Sprintf("Subject resolution returned no results for '%s' with checksum '%s'", subject, sha256))
	}

	log.Info("Successfully resolved", len(subjects), "subjects from bundle:", strings.Join(subjects, ", "))
	return subjects, nil
}

func (c *createEvidenceCustom) createDSSEEnvelope() ([]byte, error) {
	// There's always only one subject in this case.
	envelope, err := c.createEnvelope(c.subjectRepoPaths[0], c.subjectSha256)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}

func (c *createEvidenceCustom) validateSubject(subjectRepoPath string) error {
	// Pattern: must have at least one slash with non-empty sections
	if matched, _ := regexp.MatchString(`^[^/]+(/[^/]+)+$`, subjectRepoPath); !matched {
		return c.newSubjectError("Subject '" + subjectRepoPath + "' is invalid. Subject must be in format: <repo>/<path>/<name> or <repo>/<name>")
	}
	return nil
}

func (c *createEvidenceCustom) handleSubjectNotFound(subjectRepoPath string, err error) error {
	errStr := err.Error()
	if strings.Contains(errStr, "404 Not Found") {
		log.Debug("Server response error:", err.Error())
		return c.newSubjectError("Subject '" + subjectRepoPath + "' is not found. Please ensure the subject exists.")
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

func (c *createEvidenceCustom) recordSummary(subjectRepoPath string, response *model.CreateResponse) {
	commandSummary := commandsummary.EvidenceSummaryData{
		Subject:       subjectRepoPath,
		SubjectSha256: c.subjectSha256,
		PredicateType: response.PredicateType,
		PredicateSlug: response.PredicateSlug,
		Verified:      response.Verified,
		DisplayName:   subjectRepoPath,
		SubjectType:   commandsummary.SubjectTypeArtifact,
	}
	err := c.recordEvidenceSummary(commandSummary)
	if err != nil {
		log.Warn("Failed to record evidence summary:", err.Error())
	}
}

func (c *createEvidenceCustom) determineFinalError(errors []error, successfulSubjects, failedSubjects []string) error {
	if len(errors) == 0 {
		return nil
	}

	// If auto subject resolution is enabled, use NoOp exit code for subject-related failures
	if c.autoSubjectResolution {
		errorMsg := fmt.Sprintf("Failed to process %d subjects: %s", len(failedSubjects), strings.Join(failedSubjects, ", "))
		if len(successfulSubjects) > 0 {
			errorMsg = fmt.Sprintf("Partially successful: %d succeeded, %d failed. Failed subjects: %s",
				len(successfulSubjects), len(failedSubjects), strings.Join(failedSubjects, ", "))
		}
		return coreutils.CliError{
			ExitCode: coreutils.ExitCodeFailNoOp,
			ErrorMsg: errorMsg,
		}
	}

	// For manual subject specification (single subject), return the error directly
	// Manual mode only has one subject, so no partial success scenario
	return errors[0]
}
