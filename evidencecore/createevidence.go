package evidencecore

import (
	"encoding/json"
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecore/cryptolib"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecore/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecore/intoto"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"os"
	"strings"
)

const minimalLifecycleArtifactoryVersion = "7.80.0"

type EvidenceCreateCommand struct {
	serverDetails     *config.ServerDetails
	predicateFilePath string
	predicateType     string
	subjects          string
	key               string
	keyId             string
	evidenceName      string
	override          bool
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

func (ec *EvidenceCreateCommand) SetSubjects(subjects string) *EvidenceCreateCommand {
	ec.subjects = subjects
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

func (ec *EvidenceCreateCommand) SetEvidenceName(evidenceName string) *EvidenceCreateCommand {
	ec.evidenceName = evidenceName
	return ec
}

func (ec *EvidenceCreateCommand) SetOverride(override bool) *EvidenceCreateCommand {
	ec.override = override
	return ec
}

func (ec *EvidenceCreateCommand) CommandName() string {
	return "create-evidencecore"
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
	if errorutils.CheckError(err) != nil {
		return err
	}
	servicesManager, err := utils.CreateUploadServiceManager(serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return err
	}

	intotoStatement := intoto.NewStatement(predicate, ec.predicateType)
	err = intotoStatement.SetSubject(servicesManager, ec.subjects)
	if err != nil {
		return err
	}
	intotoJson, err := intotoStatement.Marshal()
	if err != nil {
		return err
	}

	// Load private key from file
	keyFile, err := os.ReadFile(ec.key)
	if err != nil {
		return err
	}

	privateKey, err := cryptolib.ReadKey(keyFile)
	if err != nil {
		return err
	}
	// If keyId is provided, use it to the single key in the privateKeys slice
	if ec.keyId != "" {
		(*privateKey).KeyID = ec.keyId
	}

	var signers []dsse.Signer

	// create actual singers
	if privateKey.KeyType == cryptolib.ECDSAKeyType {
		ecdsaSinger, err := cryptolib.NewECDSASignerVerifierFromSSLibKey(privateKey)
		if err != nil {
			return err
		}
		signers = append(signers, ecdsaSinger)
	} else if privateKey.KeyType == cryptolib.RSAKeyType {
		rsaSinger, err := cryptolib.NewRSAPSSSignerVerifierFromSSLibKey(privateKey)
		if err != nil {
			return err
		}
		signers = append(signers, rsaSinger)
	} else if privateKey.KeyType == cryptolib.ED25519KeyType {
		ed25519Singer, err := cryptolib.NewED25519SignerVerifierFromSSLibKey(privateKey)
		if err != nil {
			return err
		}
		signers = append(signers, ed25519Singer)
	} else {
		return errors.New("unsupported key type")
	}

	// Use the signers to create an envelope signer
	envelopeSigner, err := dsse.NewEnvelopeSigner(signers...)
	if err != nil {
		return err
	}

	// Iterate over all the signers and sign the dsse envelope
	signedEnvelope, err := envelopeSigner.SignPayload(intoto.DssePayloadType, intotoJson)
	if err != nil {
		return err
	}

	// create tmp dir for create evidencecore file and save dsse there
	tempDirPath, err := fileutils.CreateTempDir()
	if err != nil {
		return err
	}
	// Cleanup the temp working directory at the end.
	defer func() {
		err = errors.Join(err, fileutils.RemoveTempDir(tempDirPath))
	}()

	// Create the evidence file.
	evdName := "/evidence.json.evd"
	if ec.evidenceName != "" {
		evdName = "/" + ec.evidenceName + ".json.evd"
	}
	evidenceFilePath := tempDirPath + evdName
	evidenceFile, err := os.Create(evidenceFilePath)
	if err != nil {
		return err
	}
	defer evidenceFile.Close()

	// Encode signedEnvelope into a byte slice
	envelopeBytes, err := json.Marshal(signedEnvelope)
	if err != nil {
		return err
	}

	// Write the encoded byte slice to the file
	_, err = evidenceFile.Write(envelopeBytes)
	if err != nil {
		return err
	}

	// upload evidencecore file to artifactory
	commonParams := rtServicesUtils.CommonParams{
		Pattern: evidenceFilePath,
		Target:  strings.Split(intotoStatement.Subject[0].Uri, "/")[0] + "/",
	}
	var uploadParamsArray []services.UploadParams
	uploadParamsArray = append(uploadParamsArray, services.UploadParams{
		CommonParams: &commonParams,
		Flat:         true,
	})
	_, _, err = servicesManager.UploadFiles(uploadParamsArray...)
	if err != nil {
		return err
	}

	return nil
}
