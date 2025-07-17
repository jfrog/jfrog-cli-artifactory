package verify

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
	"io"
	"os"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	clientLog "github.com/jfrog/jfrog-client-go/utils/log"
)

const localKeySource = "User Provided Key"
const artifactoryKeySource = "Artifactory Key"

// EvidenceVerifierInterface defines the interface for evidence verification
type EvidenceVerifierInterface interface {
	Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error)
}

type evidenceVerifier struct {
	keys               []string
	useArtifactoryKeys bool
	artifactoryClient  artifactory.ArtifactoryServicesManager
	localKeys          []dsse.Verifier
}

func NewEvidenceVerifier(keys []string, useArtifactoryKeys bool, client *artifactory.ArtifactoryServicesManager) EvidenceVerifierInterface {
	return &evidenceVerifier{
		keys:               keys,
		artifactoryClient:  *client,
		useArtifactoryKeys: useArtifactoryKeys,
	}
}

// Verify checks the subject against evidence and verifies signatures using local and remote keys.
func (v *evidenceVerifier) Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error) {
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
func (v *evidenceVerifier) verifyEvidence(evidence *model.SearchEvidenceEdge, subjectSha256 string) (*model.EvidenceVerification, error) {
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
		DsseEnvelope:    envelope,
		EvidencePath:    evidence.Node.DownloadPath,
		SubjectChecksum: evidenceChecksum,
		PredicateType:   evidence.Node.PredicateType,
		CreatedBy:       evidence.Node.CreatedBy,
		Time:            evidence.Node.CreatedAt,
		VerificationResult: model.EvidenceVerificationResult{
			ChecksumVerificationStatus:   checksumStatus,
			SignaturesVerificationStatus: model.Failed,
		},
	}
	localVerifiers, err := v.getLocalVerifiers()
	if err != nil && v.keys != nil && len(v.keys) > 0 {
		return nil, err
	}
	if len(localVerifiers) > 0 && verifyEnvelope(localVerifiers, &envelope, result) {
		result.VerificationResult.KeySource = localKeySource
		return result, nil
	}

	// If verification is restricted to local keys, return the result early.
	if !v.useArtifactoryKeys {
		return result, nil
	}
	artifactoryVerifiers, err := getArtifactoryVerifiers(evidence)
	if err != nil {
		return nil, err
	}
	if verifyEnvelope(*artifactoryVerifiers, &envelope, result) {
		result.VerificationResult.KeySource = artifactoryKeySource
		return result, nil
	}
	return result, nil
}

// verifyEnvelope returns true if verification succeeded, false otherwise. Uses pointer for result.
func verifyEnvelope(verifiers []dsse.Verifier, envelope *dsse.Envelope, result *model.EvidenceVerification) bool {
	if verifiers == nil || result == nil || envelope == nil {
		return false
	}
	for _, verifier := range verifiers {
		if err := envelope.Verify(verifier); err == nil {
			result.VerificationResult.SignaturesVerificationStatus = model.Success
			fingerprint, err := cryptox.GenerateFingerprint(verifier.Public())
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

func (v *evidenceVerifier) getLocalVerifiers() ([]dsse.Verifier, error) {
	if v.localKeys != nil {
		return v.localKeys, nil
	}
	var keys []dsse.Verifier
	for _, keyPath := range v.keys {
		keyFile, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read key %s: %w", keyPath, err)
		}
		loadedKey, err := cryptox.ReadPublicKey(keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load key %s: %w", keyPath, err)
		}
		if loadedKey == nil {
			return nil, fmt.Errorf("key is null or empty %s", keyPath)
		}
		verifier, err := cryptox.CreateVerifier(loadedKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create verifier for key %s: %w", keyPath, err)
		}
		keys = append(keys, verifier...)
	}
	v.localKeys = keys
	return keys, nil
}

func (v *evidenceVerifier) readEnvelope(evidence model.SearchEvidenceEdge) (dsse.Envelope, error) {
	file, err := v.artifactoryClient.ReadRemoteFile(evidence.Node.DownloadPath)
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

func getArtifactoryVerifiers(evidence *model.SearchEvidenceEdge) (*[]dsse.Verifier, error) {
	evidenceSigningKey := evidence.Node.SigningKey
	if evidenceSigningKey.PublicKey == "" {
		return nil, fmt.Errorf("evidence artifactory key is missing for evidence predicate type: %s", evidence.Node.PredicateType)
	}
	key, err := cryptox.LoadKey([]byte(evidenceSigningKey.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to load artifactory key: %w", err)
	}
	verifier, err := cryptox.CreateVerifier(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier for evidence predicate type: %s", evidence.Node.PredicateType)
	}
	return &verifier, nil
}
