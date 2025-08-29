package verifiers

import (
	"fmt"
	"os"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/cryptox"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	clientLog "github.com/jfrog/jfrog-client-go/utils/log"
)

const localKeySource = "User Provided Key"
const artifactoryKeySource = "Artifactory Key"

type dsseVerifierInterface interface {
	verify(evidence *model.SearchEvidenceEdge, result *model.EvidenceVerification) error
}

type dsseVerifier struct {
	keys               []string
	useArtifactoryKeys bool
	localKeys          []dsse.Verifier
}

func newDsseVerifier(keys []string, useArtifactoryKeys bool) dsseVerifierInterface {
	return &dsseVerifier{
		keys:               keys,
		useArtifactoryKeys: useArtifactoryKeys,
	}
}

func (v *dsseVerifier) verify(evidence *model.SearchEvidenceEdge, result *model.EvidenceVerification) error {
	if evidence == nil || result == nil {
		return fmt.Errorf("empty evidence or result provided for DSSE verification")
	}
	localVerifiers, err := v.getLocalVerifiers()
	if err != nil && v.keys != nil && len(v.keys) > 0 {
		return err
	}
	if len(localVerifiers) > 0 && verifyEnvelope(localVerifiers, result.DsseEnvelope, result) {
		result.VerificationResult.KeySource = localKeySource
		return nil
	}

	// If verification is restricted to local keys, return the result early.
	if !v.useArtifactoryKeys {
		return nil
	}
	artifactoryVerifiers, err := getArtifactoryVerifiers(evidence)
	if err != nil {
		return err
	}
	if verifyEnvelope(artifactoryVerifiers, result.DsseEnvelope, result) {
		result.VerificationResult.KeySource = artifactoryKeySource
		return nil
	}
	return nil
}

func (v *dsseVerifier) getLocalVerifiers() ([]dsse.Verifier, error) {
	if v.localKeys != nil {
		return v.localKeys, nil
	}
	var keys []dsse.Verifier
	for _, keyPath := range v.keys {
		if keyPath == "" {
			continue
		}
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

// verifyEnvelope returns true if verification succeeded, false otherwise. Uses pointer for result.
func verifyEnvelope(verifiers []dsse.Verifier, envelope *dsse.Envelope, result *model.EvidenceVerification) bool {
	// formal check for empty result
	if result == nil {
		return false
	}
	if verifiers == nil || envelope == nil {
		result.VerificationResult.SignaturesVerificationStatus = model.Failed
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

func getArtifactoryVerifiers(evidence *model.SearchEvidenceEdge) ([]dsse.Verifier, error) {
	if evidence == nil {
		return nil, fmt.Errorf("empty evidence provided for artifactory verifier retrieval")
	}
	evidenceSigningKey := evidence.Node.SigningKey
	if evidenceSigningKey.PublicKey == "" {
		return []dsse.Verifier{}, nil
	}
	key, err := cryptox.LoadKey([]byte(evidenceSigningKey.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to load artifactory key: %w", err)
	}
	verifier, err := cryptox.CreateVerifier(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier for evidence predicate type: %s", evidence.Node.PredicateType)
	}
	return verifier, nil
}
