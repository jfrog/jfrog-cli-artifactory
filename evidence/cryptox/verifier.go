package cryptox

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"io"
	"os"
)

type Verifier struct {
	keys              []string
	artifactoryClient artifactory.ArtifactoryServicesManager
	localKeys         []dsse.Verifier
	trustedVerifiers  []dsse.Verifier
	keypairVerifiers  []dsse.Verifier
}

func NewVerifier(keys []string, client *artifactory.ArtifactoryServicesManager) *Verifier {
	return &Verifier{
		keys:              keys,
		artifactoryClient: *client,
	}
}

// Verify checks the subject against evidence and verifies signatures using local and remote keys.
func (v *Verifier) Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge) (*model.VerificationResponse, error) {
	if evidenceMetadata == nil || len(*evidenceMetadata) == 0 {
		return nil, fmt.Errorf("no evidence metadata provided")
	}
	result := &model.VerificationResponse{
		Checksum: subjectSha256,
	}
	results := make([]model.EvidenceVerificationResult, 0, len(*evidenceMetadata))
	for i := range *evidenceMetadata {
		evidence := &(*evidenceMetadata)[i]
		verificationResult, err := v.verifyEvidence(evidence, subjectSha256)
		if err != nil {
			return nil, err
		}
		results = append(results, *verificationResult)
		if verificationResult.SignaturesVerificationStatus == model.Failed || verificationResult.ChecksumVerificationStatus == model.Failed {
			result.OverallVerificationStatus = model.Failed
		}
	}
	result.EvidencesVerificationResults = &results
	if result.OverallVerificationStatus != model.Failed {
		result.OverallVerificationStatus = model.Success
	}
	return result, nil
}

// verifyEvidence verifies a single evidence using local and remote keys, avoiding unnecessary copies.
func (v *Verifier) verifyEvidence(evidence *model.SearchEvidenceEdge, subjectSha256 string) (*model.EvidenceVerificationResult, error) {
	if evidence == nil {
		return nil, fmt.Errorf("nil evidence provided")
	}
	var checksumStatus string
	evidenceChecksum := evidence.Node.Subject.Sha256
	if subjectSha256 != evidenceChecksum {
		checksumStatus = model.Failed
	} else {
		checksumStatus = model.Success
	}
	envelope, err := v.readEnvelop(*evidence)
	if err != nil {
		return nil, fmt.Errorf("failed to read envelope: %w", err)
	}
	result := &model.EvidenceVerificationResult{
		Checksum:                     evidenceChecksum,
		ChecksumVerificationStatus:   checksumStatus,
		EvidenceType:                 evidence.Node.PredicateType,
		Category:                     evidence.Node.PredicateCategory,
		CreatedBy:                    evidence.Node.CreatedBy,
		Time:                         evidence.Node.CreatedAt,
		SignaturesVerificationStatus: model.Failed, // Default to Failed
	}
	localVerifiers, err := v.getKeyFiles()
	if err != nil && v.keys != nil && len(v.keys) > 0 {
		return nil, err
	}
	if len(localVerifiers) > 0 && verifyEnvelop(localVerifiers, &envelope, result) {
		return result, nil
	}
	// Always try both trusted and keypair keys for each evidence
	trustedVerifiers, err := v.getTrustedVerifiers()
	if err == nil && len(trustedVerifiers) > 0 && verifyEnvelop(trustedVerifiers, &envelope, result) {
		return result, nil
	}
	keypairVerifiers, err := v.getKeypairVerifiers()
	if err == nil && len(keypairVerifiers) > 0 && verifyEnvelop(keypairVerifiers, &envelope, result) {
		return result, nil
	}
	return result, nil
}

// verifyEnvelop returns true if verification succeeded, false otherwise. Uses pointer for result.
func verifyEnvelop(verifiers []dsse.Verifier, envelope *dsse.Envelope, result *model.EvidenceVerificationResult) bool {
	if verifiers == nil || result == nil || envelope == nil {
		return false
	}
	for _, verifier := range verifiers {
		if err := envelope.Verify(verifier); err == nil {
			result.SignaturesVerificationStatus = model.Success
			return true
		}
	}
	result.SignaturesVerificationStatus = model.Failed
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

func (v *Verifier) getKeyFiles() ([]dsse.Verifier, error) {
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

func (v *Verifier) readEnvelop(evidence model.SearchEvidenceEdge) (dsse.Envelope, error) {
	return readEnvelopeFromRemote(evidence.Node.DownloadPath, v.artifactoryClient)
}

func (v *Verifier) getTrustedVerifiers() ([]dsse.Verifier, error) {
	if v.trustedVerifiers != nil {
		return v.trustedVerifiers, nil
	}
	trustedVerifiers, err := FetchTrustedKeys(v.artifactoryClient)
	if err == nil {
		v.trustedVerifiers = trustedVerifiers
	}
	return trustedVerifiers, err
}

func (v *Verifier) getKeypairVerifiers() ([]dsse.Verifier, error) {
	if v.keypairVerifiers != nil {
		return v.keypairVerifiers, nil
	}
	keypairVerifiers, err := FetchKeyPairs(v.artifactoryClient)
	if err == nil {
		v.keypairVerifiers = keypairVerifiers
	}
	return keypairVerifiers, err
}
