package cryptox

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	clientLog "github.com/jfrog/jfrog-client-go/utils/log"
)

const localKeySource = "Local Key"
const artifactoryPublicKeySource = "Artifactory Public Key"
const artifactorySigningKeySource = "Artifactory Signing Key"

// EvidenceVerifierInterface defines the interface for evidence verification
type EvidenceVerifierInterface interface {
	Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error)
}

type EvidenceVerifier struct {
	keys               []string
	useArtifactoryKeys bool
	artifactoryClient  artifactory.ArtifactoryServicesManager
	localKeys          []dsse.Verifier
	trustedVerifiers   []dsse.Verifier
	keypairVerifiers   []dsse.Verifier
}

func NewEvidenceVerifier(keys []string, useArtifactoryKeys bool, client *artifactory.ArtifactoryServicesManager) *EvidenceVerifier {
	return &EvidenceVerifier{
		keys:               keys,
		artifactoryClient:  *client,
		useArtifactoryKeys: useArtifactoryKeys,
	}
}

// Verify checks the subject against evidence and verifies signatures using local and remote keys.
func (v *EvidenceVerifier) Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error) {
	if evidenceMetadata == nil || len(*evidenceMetadata) == 0 {
		return nil, fmt.Errorf("no evidence metadata provided")
	}
	result := &model.VerificationResponse{
		SubjectPath:               subjectPath,
		SubjectChecksum:           subjectSha256,
		OverallVerificationStatus: model.Success,
	}
	results := make([]model.EvidenceVerification, 0, len(*evidenceMetadata))
	for i := range *evidenceMetadata {
		evidence := &(*evidenceMetadata)[i]
		verification, err := v.verifyEvidence(evidence, subjectSha256)
		if err != nil {
			return nil, err
		}
		results = append(results, *verification)
		if verification.VerificationResult.SignaturesVerificationStatus == model.Failed || verification.VerificationResult.ChecksumVerificationStatus == model.Failed {
			result.OverallVerificationStatus = model.Failed
		}
	}
	result.EvidenceVerifications = &results
	return result, nil
}

// verifyEvidence verifies a single evidence using local and remote keys, avoiding unnecessary copies.
func (v *EvidenceVerifier) verifyEvidence(evidence *model.SearchEvidenceEdge, subjectSha256 string) (*model.EvidenceVerification, error) {
	if evidence == nil {
		return nil, fmt.Errorf("nil evidence provided")
	}
	envelope, err := v.readEnvelope(*evidence)
	if err != nil {
		return nil, fmt.Errorf("failed to read envelope: %w", err)
	}
	var checksumStatus model.VerificationStatus
	evidenceChecksum := evidence.Node.Subject.Sha256
	if subjectSha256 != evidenceChecksum {
		checksumStatus = model.Failed
	} else {
		checksumStatus = model.Success
	}
	result := &model.EvidenceVerification{
		DsseEnvelope:  envelope,
		EvidencePath:  evidence.Node.DownloadPath,
		Checksum:      evidenceChecksum,
		PredicateType: evidence.Node.PredicateType,
		CreatedBy:     evidence.Node.CreatedBy,
		Time:          evidence.Node.CreatedAt,
		VerificationResult: model.EvidenceVerificationResult{
			ChecksumVerificationStatus:   checksumStatus,
			SignaturesVerificationStatus: model.Failed,
		},
	}
	localVerifiers, err := v.getKeyFiles()
	if err != nil && v.keys != nil && len(v.keys) > 0 {
		return nil, err
	}
	if len(localVerifiers) > 0 && verifyEnvelop(localVerifiers, &envelope, result) {
		result.VerificationResult.KeySource = localKeySource
		return result, nil
	}

	// If verification is restricted to local keys, return the result early.
	if !v.useArtifactoryKeys {
		return result, nil
	}

	trustedVerifiers, err := v.getTrustedVerifiers()
	if err != nil {
		return nil, err
	}
	if len(trustedVerifiers) > 0 && verifyEnvelop(trustedVerifiers, &envelope, result) {
		result.VerificationResult.KeySource = artifactoryPublicKeySource
		return result, nil
	}
	keypairVerifiers, err := v.getKeypairVerifiers()
	if err != nil {
		return nil, err
	}
	if len(keypairVerifiers) > 0 && verifyEnvelop(keypairVerifiers, &envelope, result) {
		result.VerificationResult.KeySource = artifactorySigningKeySource
		return result, nil
	}
	return result, nil
}

// verifyEnvelop returns true if verification succeeded, false otherwise. Uses pointer for result.
func verifyEnvelop(verifiers []dsse.Verifier, envelope *dsse.Envelope, result *model.EvidenceVerification) bool {
	if verifiers == nil || result == nil || envelope == nil {
		return false
	}
	for _, verifier := range verifiers {
		if err := envelope.Verify(verifier); err == nil {
			result.VerificationResult.SignaturesVerificationStatus = model.Success
			fingerprint, err := GenerateFingerprint(verifier.Public())
			if err != nil {
				clientLog.Warn("Failed to generate fingerprint for the key: %s", verifier.Public())
			} else {
				result.VerificationResult.KeyFingerprint = fingerprint
			}
			return true
		}
	}
	result.VerificationResult.SignaturesVerificationStatus = model.Failed
	return false
}

// readEnvelopeFromRemote reads and unmarshals a DSSE envelope from a remote path using the Artifactory client.
func readEnvelopeFromRemote(downloadPath string, client artifactory.ArtifactoryServicesManager) (dsse.Envelope, error) {
	file, err := client.ReadRemoteFile(downloadPath)
	if err != nil {
		return dsse.Envelope{}, fmt.Errorf("failed to read remote file: %w", err)
	}
	defer func(file io.ReadCloser) {
		_ = file.Close()
	}(file)
	fileContent, err := io.ReadAll(file)
	if err != nil {
		return dsse.Envelope{}, fmt.Errorf("failed to read file content: %w", err)
	}
	envelope := dsse.Envelope{}
	err = json.Unmarshal(fileContent, &envelope)
	if err != nil {
		return dsse.Envelope{}, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}
	return envelope, nil
}

func (v *EvidenceVerifier) getKeyFiles() ([]dsse.Verifier, error) {
	if v.localKeys != nil {
		return v.localKeys, nil
	}
	var keys []dsse.Verifier
	for _, keyPath := range v.keys {
		keyFile, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read key %s", keyPath)
		}
		loadedKey, err := ReadPublicKey(keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load key %s: %w", keyPath, err)
		}
		if loadedKey == nil {
			return nil, fmt.Errorf("failed to load key %s", keyPath)
		}
		verifier, err := createVerifier(loadedKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create verifier for key %s: %w", keyPath, err)
		}
		keys = append(keys, verifier...)
	}
	v.localKeys = keys
	return keys, nil
}

func (v *EvidenceVerifier) readEnvelope(evidence model.SearchEvidenceEdge) (dsse.Envelope, error) {
	return readEnvelopeFromRemote(evidence.Node.DownloadPath, v.artifactoryClient)
}

func (v *EvidenceVerifier) getTrustedVerifiers() ([]dsse.Verifier, error) {
	if v.trustedVerifiers != nil {
		return v.trustedVerifiers, nil
	}
	trustedVerifiers, err := FetchTrustedKeys(v.artifactoryClient)
	if err == nil {
		v.trustedVerifiers = trustedVerifiers
	}
	return trustedVerifiers, err
}

func (v *EvidenceVerifier) getKeypairVerifiers() ([]dsse.Verifier, error) {
	if v.keypairVerifiers != nil {
		return v.keypairVerifiers, nil
	}
	keypairVerifiers, err := FetchKeyPairs(v.artifactoryClient)
	if err == nil {
		v.keypairVerifiers = keypairVerifiers
	}
	return keypairVerifiers, err
}
