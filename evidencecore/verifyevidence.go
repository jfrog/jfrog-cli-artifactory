package evidencecore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecore/cryptolib"
	"github.com/jfrog/jfrog-cli-artifactory/evidencecore/dsse"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"net/url"
	"os"
)

type EvidenceVerifyCommand struct {
	serverDetails *config.ServerDetails
	key           string
	evidenceName  string
}

func NewEvidenceVerifyCommand() *EvidenceVerifyCommand {
	return &EvidenceVerifyCommand{}
}

func (evc *EvidenceVerifyCommand) SetServerDetails(serverDetails *config.ServerDetails) *EvidenceVerifyCommand {
	evc.serverDetails = serverDetails
	return evc
}

func (evc *EvidenceVerifyCommand) SetKey(key string) *EvidenceVerifyCommand {
	evc.key = key
	return evc
}

func (evc *EvidenceVerifyCommand) SetEvidenceName(evidenceName string) *EvidenceVerifyCommand {
	evc.evidenceName = evidenceName
	return evc
}

func (evc *EvidenceVerifyCommand) CommandName() string {
	return "verify_create"
}

func (evc *EvidenceVerifyCommand) ServerDetails() (*config.ServerDetails, error) {
	return evc.serverDetails, nil
}

func (evc *EvidenceVerifyCommand) Run() error {
	u, err := url.Parse(evc.evidenceName)
	if err != nil {
		return err
	}
	var key []byte
	// Check if the evidence name is a URL
	if u.Scheme != "" || u.Host != "" {
		// Load evidence from Artifactory
		// Create services manager
		serverDetails, err := evc.ServerDetails()
		if errorutils.CheckError(err) != nil {
			return err
		}
		downloadServicemanager, err := utils.CreateDownloadServiceManager(serverDetails, 1, 0, 0, false, nil)
		if err != nil {
			return err
		}
		// Download evidence from Artifactory

		tempDirPath, err := fileutils.CreateTempDir()
		if err != nil {
			return err
		}
		// Cleanup the temp working directory at the end.
		defer func() {
			err = errors.Join(err, fileutils.RemoveTempDir(tempDirPath))
		}()

		commonParams := rtServicesUtils.CommonParams{
			Pattern: evc.evidenceName, // Path in the repository generic-local/aaa/asdfd.json.evd
			Target:  tempDirPath,      // Save to local file system
		}
		var downloadParamsArray []services.DownloadParams
		downloadParamsArray = append(downloadParamsArray, services.DownloadParams{
			CommonParams: &commonParams,
			Flat:         true,
		})
		totalDownloaded, _, err := downloadServicemanager.DownloadFiles(downloadParamsArray...)
		if err != nil {
			return err
		}
		if totalDownloaded != 1 {
			return errors.New("failed to download evidence")
		}
		// Load evidence from file system
		dsseFile, err := os.ReadFile(tempDirPath + "/" + u.Path)
		dsseFile = dsseFile
	}
	// We assume that the evidence name is a path, so we can assume that it is a local file
	key, err = os.ReadFile(evc.key)
	if err != nil {
		return err
	}
	// Load evidence from file system
	dsseFile, err := os.ReadFile(evc.evidenceName)
	if err != nil {
		return err
	}
	// Load key from file
	loadedKey, err := cryptolib.LoadKey(key)
	if err != nil {
		return err
	}
	// Verify evidence with key
	dsseEnvelope := dsse.Envelope{}
	err = json.Unmarshal(dsseFile, &dsseEnvelope)
	if err != nil {
		return err
	}

	// Decode payload and key
	decodedPayload, err := base64.StdEncoding.DecodeString(dsseEnvelope.Payload)
	if err != nil {
		return err
	}
	decodedKey, err := base64.StdEncoding.DecodeString(dsseEnvelope.Signatures[0].Sig) // This stage we support only one signature
	if err != nil {
		return err
	}

	// Create PAE
	paeEnc := dsse.PAE(dsseEnvelope.PayloadType, decodedPayload)

	// create actual verifier
	if loadedKey.KeyType == cryptolib.ECDSAKeyType {
		ecdsaSinger, err := cryptolib.NewECDSASignerVerifierFromSSLibKey(loadedKey)
		if err != nil {
			return err
		}
		err = ecdsaSinger.Verify(paeEnc, decodedKey)
		if err != nil {
			return err
		}
	} else if loadedKey.KeyType == cryptolib.RSAKeyType {
		rsaSinger, err := cryptolib.NewRSAPSSSignerVerifierFromSSLibKey(loadedKey)
		if err != nil {
			return err
		}
		err = rsaSinger.Verify(paeEnc, decodedKey)
		if err != nil {
			return err
		}
	} else if loadedKey.KeyType == cryptolib.ED25519KeyType {
		ed25519Singer, err := cryptolib.NewED25519SignerVerifierFromSSLibKey(loadedKey)
		if err != nil {
			return err
		}
		err = ed25519Singer.Verify(paeEnc, decodedKey)
		if err != nil {
			return err
		}
	} else {
		return errors.New("unsupported key type")
	}

	return nil
}
