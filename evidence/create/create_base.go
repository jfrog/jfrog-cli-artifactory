package create

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/sonar"
	evidenceUtils "github.com/jfrog/jfrog-cli-artifactory/evidence/utils"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/intoto"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/sign"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	evidenceService "github.com/jfrog/jfrog-client-go/evidence/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const EvdDefaultUser = "JFrog CLI"

type createEvidenceBase struct {
	serverDetails     *config.ServerDetails
	predicateFilePath string
	predicateType     string
	markdownFilePath  string
	markdown          []byte
	key               string
	keyId             string
	providerId        string
	stage             string
	flagType          FlagType
	useSonarPredicate bool
}

func (c *createEvidenceBase) createEnvelope(subject, subjectSha256 string) ([]byte, error) {
	var statementJson []byte
	var err error
	if c.useSonarPredicate {
		statementJson, err = c.buildStatementFromSonar(subject, subjectSha256)
		if err != nil {
			log.Debug("Main statement flow failed, falling back to predicate flow:", err.Error())
			sonarPredicate, perr := c.buildSonarPredicate()
			if perr != nil {
				return nil, perr
			}
			statementJson, err = c.buildIntotoStatementJson(subject, subjectSha256, sonarPredicate)
			if err != nil {
				return nil, err
			}
		}
	} else {
		statementJson, err = c.buildIntotoStatementJson(subject, subjectSha256, nil)
	}
	if err != nil {
		return nil, err
	}
	signedEnvelope, err := createAndSignEnvelope(statementJson, c.key, c.keyId)
	if err != nil {
		return nil, err
	}
	envelopeBytes, err := json.Marshal(signedEnvelope)
	if err != nil {
		return nil, err
	}
	return envelopeBytes, nil
}

func (c *createEvidenceBase) buildSonarPredicate() ([]byte, error) {
	resolver := sonar.NewPredicateResolver()
	predicateType, predicate, markdown, err := resolver.ResolvePredicate()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve predicate: %w", err)
	}
	c.predicateType = predicateType
	c.providerId = "sonar"
	if len(markdown) > 0 {
		c.markdown = markdown
	}
	return predicate, nil
}

// buildStatementFromSonar fetches in-toto statement from main flow, augments it with subject and stage, and returns it.
func (c *createEvidenceBase) buildStatementFromSonar(subject, subjectSha256 string) ([]byte, error) {
	stmtResolver := sonar.NewStatementResolver()
	statementBytes, _, err := stmtResolver.ResolveStatement()
	if err != nil {
		return nil, err
	}
	sha := subjectSha256
	if sha == "" {
		artifactoryClient, cerr := c.createArtifactoryClient()
		if cerr != nil {
			return nil, cerr
		}
		cs, cerr := c.getFileChecksum(subject, artifactoryClient)
		if cerr != nil {
			return nil, cerr
		}
		sha = cs
	}
	augmented, err := augmentStatementWithSubjectAndStage(statementBytes, sha, c.stage)
	if err != nil {
		return nil, err
	}
	c.providerId = "sonar"
	return augmented, nil
}

func (c *createEvidenceBase) createEnvelopeWithPredicateAndPredicateType(subject,
	subjectSha256, predicateType string, predicate []byte) ([]byte, error) {
	statementJson, err := c.buildIntotoStatementJsonWithPredicateAndPredicateType(subject,
		subjectSha256, predicateType, predicate)
	if err != nil {
		return nil, err
	}

	signedEnvelope, err := createAndSignEnvelope(statementJson, c.key, c.keyId)
	if err != nil {
		return nil, err
	}

	// Encode signedEnvelope into a byte slice
	envelopeBytes, err := json.Marshal(signedEnvelope)
	if err != nil {
		return nil, err
	}
	return envelopeBytes, nil
}

func (c *createEvidenceBase) buildIntotoStatementJson(subject, subjectSha256 string, predicate []byte) ([]byte, error) {
	if len(predicate) == 0 {
		customPredicate, err := os.ReadFile(c.predicateFilePath)
		if err != nil {
			log.Warn(fmt.Sprintf("failed to read predicate file '%s'", customPredicate))
			return nil, err
		}
		predicate = customPredicate
	}

	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		return nil, err
	}

	user := c.serverDetails.User
	if user == "" {
		user = EvdDefaultUser
	}

	statement := intoto.NewStatement(predicate, c.predicateType, user)
	err = c.setMarkdown(statement)
	if err != nil {
		return nil, err
	}

	err = statement.SetSubject(artifactoryClient, subject, subjectSha256)
	if err != nil {
		return nil, err
	}
	statement.SetStage(c.stage)
	statementJson, err := statement.Marshal()
	if err != nil {
		log.Error("failed marshaling statement json file", err)
		return nil, err
	}
	return statementJson, nil
}

func (c *createEvidenceBase) buildIntotoStatementJsonWithPredicateAndPredicateType(subject, subjectSha256, predicateType string, predicate []byte) ([]byte, error) {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		return nil, err
	}

	statement := intoto.NewStatement(predicate, predicateType, c.serverDetails.User)
	err = c.setMarkdown(statement)
	if err != nil {
		return nil, err
	}

	err = statement.SetSubject(artifactoryClient, subject, subjectSha256)
	if err != nil {
		return nil, err
	}
	statementJson, err := statement.Marshal()
	if err != nil {
		log.Error("failed marshaling statement json file", err)
		return nil, err
	}
	return statementJson, nil
}

func (c *createEvidenceBase) setMarkdown(statement *intoto.Statement) error {
	if len(c.markdown) > 0 {
		statement.SetMarkdown(c.markdown)
		return nil
	}

	if c.markdownFilePath != "" {
		if !strings.HasSuffix(c.markdownFilePath, ".md") {
			return fmt.Errorf("file '%s' does not have a .md extension", c.markdownFilePath)
		}
		markdown, err := os.ReadFile(c.markdownFilePath)
		if err != nil {
			log.Warn(fmt.Sprintf("failed to read markdown file '%s'", c.markdownFilePath))
			return err
		}
		// Temporary fix: replace \n with <br> in markdown
		markdownWithBR := strings.ReplaceAll(string(markdown), "\n", "<br>")
		statement.SetMarkdown([]byte(markdownWithBR))
	}
	return nil
}

func (c *createEvidenceBase) uploadEvidence(evidencePayload []byte, repoPath string) (*model.CreateResponse, error) {
	evidenceManager, err := utils.CreateEvidenceServiceManager(c.serverDetails, false)
	if err != nil {
		return nil, err
	}

	evidenceDetails := evidenceService.EvidenceDetails{
		SubjectUri: repoPath,
		// evidencePayload may contain not only a DSSE envelop.
		DSSEFileRaw: evidencePayload,
		ProviderId:  c.providerId,
	}
	log.Info("Uploading evidence to Artifactory provider:", c.providerId)
	log.Debug("Uploading evidence for subject:", repoPath)
	body, err := evidenceManager.UploadEvidence(evidenceDetails)
	if err != nil {
		return nil, err
	}

	createResponse := &model.CreateResponse{}
	err = json.Unmarshal(body, createResponse)
	if err != nil {
		return nil, err
	}
	if createResponse.Verified {
		log.Info("Evidence successfully created and verified")
	} else {
		log.Info("Evidence successfully created but not verified due to missing/invalid public key")
	}
	return createResponse, nil
}

func (c *createEvidenceBase) recordEvidenceSummary(summaryData commandsummary.EvidenceSummaryData) error {
	if !evidenceUtils.IsRunningUnderGitHubAction() {
		return nil
	}

	evidenceSummary, err := commandsummary.NewEvidenceSummary()
	if err != nil {
		return err
	}

	return evidenceSummary.Record(summaryData)
}

func (c *createEvidenceBase) createArtifactoryClient() (artifactory.ArtifactoryServicesManager, error) {
	return utils.CreateUploadServiceManager(c.serverDetails, 1, 0, 0, false, nil)
}

func (c *createEvidenceBase) getFileChecksum(path string, artifactoryClient artifactory.ArtifactoryServicesManager) (string, error) {
	res, err := artifactoryClient.FileInfo(path)
	if err != nil {
		log.Warn(fmt.Sprintf("file path '%s' does not exist.", path))
		return "", err
	}
	return res.Checksums.Sha256, nil
}

func createAndSignEnvelope(payloadJson []byte, key string, keyId string) (*dsse.Envelope, error) {
	// Load private key from file if ec.key is not a path to a file then try to load it as a key
	keyFile := []byte(key)
	if _, err := os.Stat(key); err == nil {
		keyFile, err = os.ReadFile(key)
		if err != nil {
			return nil, err
		}
	}

	privateKey, err := cryptox.ReadKey(keyFile)
	if err != nil {
		return nil, err
	}

	if privateKey == nil {
		return nil, errors.New("failed to load private key. please verify provided key")
	}

	privateKey.KeyID = keyId

	signers, err := createSigners(privateKey)
	if err != nil {
		return nil, err
	}

	// Use the signers to create an envelope signer
	envelopeSigner, err := sign.NewEnvelopeSigner(signers...)
	if err != nil {
		return nil, err
	}

	// Iterate over all the signers and sign the dsse envelope
	signedEnvelope, err := envelopeSigner.SignPayload(intoto.PayloadType, payloadJson)
	if err != nil {
		return nil, err
	}
	return signedEnvelope, nil
}

func createSigners(privateKey *cryptox.SSLibKey) ([]dsse.Signer, error) {
	var signers []dsse.Signer

	switch privateKey.KeyType {
	case cryptox.ECDSAKeyType:
		ecdsaSinger, err := cryptox.NewECDSASignerVerifierFromSSLibKey(privateKey)
		if err != nil {
			return nil, err
		}
		signers = append(signers, ecdsaSinger)
	case cryptox.RSAKeyType:
		rsaSinger, err := cryptox.NewRSAPSSSignerVerifierFromSSLibKey(privateKey)
		if err != nil {
			return nil, err
		}
		signers = append(signers, rsaSinger)
	case cryptox.ED25519KeyType:
		ed25519Singer, err := cryptox.NewED25519SignerVerifierFromSSLibKey(privateKey)
		if err != nil {
			return nil, err
		}
		signers = append(signers, ed25519Singer)
	default:
		return nil, errors.New("unsupported key type")
	}
	return signers, nil
}

// augmentStatementWithSubjectAndStage injects subject and stage into the given in-toto statement JSON.
func augmentStatementWithSubjectAndStage(statement []byte, sha256 string, stage string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(statement, &m); err != nil {
		return nil, err
	}
	// subject
	subject := map[string]any{
		"digest": map[string]any{
			"sha256": sha256,
		},
	}
	m["subject"] = []any{subject}
	// stage
	if stage != "" {
		m["stage"] = stage
	}
	return json.Marshal(m)
}
