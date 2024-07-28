package evidence

import (
	"encoding/json"
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/intoto"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"os"
)

type CreateEvidenceBase struct {
	serverDetails     *config.ServerDetails
	predicateFilePath string
	predicateType     string
	key               string
	keyId             string
}

func (cb *CreateEvidenceBase) createEnvelope(subject string) ([]byte, error) {
	// Load predicate from file
	predicate, err := os.ReadFile(cb.predicateFilePath)
	if err != nil {
		return nil, err
	}

	servicesManager, err := utils.CreateUploadServiceManager(cb.serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return nil, err
	}

	// Create intoto statement
	intotoStatement := intoto.NewStatement(predicate, cb.predicateType, cb.serverDetails.User)
	err = intotoStatement.SetSubject(servicesManager, subject)
	if err != nil {
		return nil, err
	}
	intotoJson, err := intotoStatement.Marshal()
	if err != nil {
		return nil, err
	}

	// Load private key from file if ec.key is not a path to a file then try to load it as a key
	keyFile := []byte(cb.key)
	if _, err := os.Stat(cb.key); err == nil {
		keyFile, err = os.ReadFile(cb.key)
		if err != nil {
			return nil, err
		}
	}

	privateKey, err := cryptox.ReadKey(keyFile)
	if err != nil {
		return nil, err
	}

	privateKey.KeyID = cb.keyId

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
	signedEnvelope, err := envelopeSigner.SignPayload(intoto.PayloadType, intotoJson)
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
