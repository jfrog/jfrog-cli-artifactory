package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/intoto"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	evidenceService "github.com/jfrog/jfrog-client-go/evidence/services"
	clientlog "github.com/jfrog/jfrog-client-go/utils/log"
	"os"
	"strings"
)

type CreateEvidenceBase struct {
	serverDetails     *config.ServerDetails
	predicateFilePath string
	predicateType     string
	key               string
	keyId             string
}

func (c *CreateEvidenceBase) createEnvelope(subject string) ([]byte, error) {
	statementJson, err := c.buildIntotoStatementJson(subject)
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

func (c *CreateEvidenceBase) buildIntotoStatementJson(subject string) ([]byte, error) {
	predicate, err := os.ReadFile(c.predicateFilePath)
	if err != nil {
		log.Warn(fmt.Sprintf("failed to read predicate file '%s'", predicate))
		return nil, err
	}

	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return nil, err
	}

	statement := intoto.NewStatement(predicate, c.predicateType, c.serverDetails.User)
	err = statement.SetSubject(artifactoryClient, subject)
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

func (c *CreateEvidenceBase) uploadEvidence(envelope []byte, repoPath string) error {
	evidenceManager, err := utils.CreateEvidenceServiceManager(c.serverDetails, false)
	if err != nil {
		log.Error("failed to create Evidence client", err)
		return err
	}

	evidenceDetails := evidenceService.EvidenceDetails{
		SubjectUri:  strings.Split(repoPath, "@")[0],
		DSSEFileRaw: envelope,
	}
	body, err := evidenceManager.UploadEvidence(evidenceDetails)
	if err != nil {
		log.Error("failed to upload evidence file", err)
		return err
	}

	createResponse := &model.CreateResponse{}
	err = json.Unmarshal(body, createResponse)
	if err != nil {
		return err
	}
	if createResponse.Verified {
		clientlog.Info("Evidence successfully created and verified")
		return nil
	}
	clientlog.Info("Evidence successfully created but not verified due to missing/invalid public key")
	return nil
}

func (c *CreateEvidenceBase) createArtifactoryClient() (artifactory.ArtifactoryServicesManager, error) {
	return utils.CreateUploadServiceManager(c.serverDetails, 1, 0, 0, false, nil)
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

	privateKey.KeyID = keyId

	signers, err := createSigners(privateKey)
	if err != nil {
		return nil, err
	}

	// Use the signers to create an envelope signer
	envelopeSigner, err := dsse.NewEnvelopeSigner(signers...)
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
