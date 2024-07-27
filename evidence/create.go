package evidence

import (
	"encoding/json"
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/intoto"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	evidenceService "github.com/jfrog/jfrog-client-go/evidence/services"
	clientlog "github.com/jfrog/jfrog-client-go/utils/log"
	"os"
	"strings"
)

type EvidenceCreateCommand struct {
	serverDetails     *config.ServerDetails
	predicateFilePath string
	predicateType     string
	repoPath          string
	key               string
	keyId             string
}

func NewEvidenceCreateCommand() *EvidenceCreateCommand {
	return &EvidenceCreateCommand{}
}

func (ec *EvidenceCreateCommand) SetServerDetails(serverDetails *config.ServerDetails) *EvidenceCreateCommand {
	ec.serverDetails = serverDetails
	return ec
}

func (ec *EvidenceCreateCommand) SetPredicateFilePath(predicateFilePath string) *EvidenceCreateCommand {
	ec.predicateFilePath = predicateFilePath
	return ec
}

func (ec *EvidenceCreateCommand) SetPredicateType(predicateType string) *EvidenceCreateCommand {
	ec.predicateType = predicateType
	return ec
}

func (ec *EvidenceCreateCommand) SetRepoPath(repoPath string) *EvidenceCreateCommand {
	ec.repoPath = repoPath
	return ec
}

func (ec *EvidenceCreateCommand) SetKey(key string) *EvidenceCreateCommand {
	ec.key = key
	return ec
}

func (ec *EvidenceCreateCommand) SetKeyId(keyId string) *EvidenceCreateCommand {
	ec.keyId = keyId
	return ec
}

func (ec *EvidenceCreateCommand) CommandName() string {
	return "create-evidence"
}

func (ec *EvidenceCreateCommand) ServerDetails() (*config.ServerDetails, error) {
	return ec.serverDetails, nil
}

func (ec *EvidenceCreateCommand) Run() error {
	// Load predicate from file
	predicate, err := os.ReadFile(ec.predicateFilePath)
	if err != nil {
		return err
	}

	// Create services manager
	serverDetails, err := ec.ServerDetails()
	if err != nil {
		return err
	}
	servicesManager, err := utils.CreateUploadServiceManager(serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return err
	}

	// Create intoto statement
	intotoStatement := intoto.NewStatement(predicate, ec.predicateType, ec.serverDetails.User)
	err = intotoStatement.SetSubject(servicesManager, ec.repoPath)
	if err != nil {
		return err
	}
	intotoJson, err := intotoStatement.Marshal()
	if err != nil {
		return err
	}

	// Load private key from file if ec.key is not a path to a file then try to load it as a key
	keyFile := []byte(ec.key)
	if _, err := os.Stat(ec.key); err == nil {
		keyFile, err = os.ReadFile(ec.key)
		if err != nil {
			return err
		}
	}

	privateKey, err := cryptox.ReadKey(keyFile)
	if err != nil {
		return err
	}

	privateKey.KeyID = ec.keyId

	signers, err := createSigners(privateKey)
	if err != nil {
		return err
	}

	// Use the signers to create an envelope signer
	envelopeSigner, err := dsse.NewEnvelopeSigner(signers...)
	if err != nil {
		return err
	}

	// Iterate over all the signers and sign the dsse envelope
	signedEnvelope, err := envelopeSigner.SignPayload(intoto.PayloadType, intotoJson)
	if err != nil {
		return err
	}

	// Encode signedEnvelope into a byte slice
	envelopeBytes, err := json.Marshal(signedEnvelope)
	if err != nil {
		return err
	}

	evidenceManager, err := utils.CreateEvidenceServiceManager(serverDetails, false)
	if err != nil {
		return err
	}

	evidenceDetails := evidenceService.EvidenceDetails{
		SubjectUri:  strings.Split(ec.repoPath, "@")[0],
		DSSEFileRaw: envelopeBytes,
	}
	body, err := evidenceManager.UploadEvidence(evidenceDetails)
	if err != nil {
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
	clientlog.Info("Evidence successfully created but not verified")
	return nil
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
