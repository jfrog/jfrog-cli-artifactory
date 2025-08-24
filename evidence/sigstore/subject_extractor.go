package sigstore

import (
	"encoding/json"
	"fmt"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

// ExtractSubjectFromBundle extracts the repository path and SHA256 checksum from a Sigstore bundle
// Returns the repository path, SHA256 checksum, and any error that occurred
func ExtractSubjectFromBundle(b *bundle.Bundle) (repoPath, sha256 string, err error) {
	if b == nil {
		return repoPath, sha256, errorutils.CheckErrorf("bundle cannot be nil")
	}

	envelope, err := GetDSSEEnvelope(b)
	if err != nil {
		return repoPath, sha256, fmt.Errorf("failed to get DSSE envelope: %w", err)
	}

	return extractSubjectFromEnvelope(envelope)
}

// extractSubjectFromEnvelope extracts the repository path and SHA256 checksum from a DSSE envelope
func extractSubjectFromEnvelope(envelope *protodsse.Envelope) (string, string, error) {
	if envelope == nil {
		return "", "", errorutils.CheckErrorf("envelope cannot be nil")
	}

	if envelope.Payload == nil {
		return "", "", errorutils.CheckErrorf("envelope payload cannot be nil")
	}

	var statement map[string]any
	if err := json.Unmarshal(envelope.Payload, &statement); err != nil {
		return "", "", errorutils.CheckErrorf("failed to parse statement from DSSE payload: %w", err)
	}

	repoPath, sha256, err := extractRepoPathFromStatement(statement)
	if err != nil {
		return "", "", err
	}
	return repoPath, sha256, nil
}

// extractRepoPathFromStatement extracts the repository path and SHA256 checksum from a statement
// The statement should contain a "subject" array with at least one subject object
// Each subject should have a "name" field and optionally a "digest" field with SHA256
func extractRepoPathFromStatement(statement map[string]any) (string, string, error) {
	if statement == nil {
		return "", "", errorutils.CheckErrorf("statement was not found in DSSE payload")
	}

	subjects, ok := statement["subject"].([]any)
	if !ok || len(subjects) == 0 {
		return "", "", errorutils.CheckErrorf("subject was not found in DSSE statement")
	}

	// Get the first subject
	subject, ok := subjects[0].(map[string]any)
	if !ok {
		return "", "", errorutils.CheckErrorf("invalid subject format in DSSE statement")
	}

	// Extract the name
	name, ok := subject["name"].(string)
	if !ok || name == "" {
		return "", "", errorutils.CheckErrorf("name was not found in DSSE subject")
	}

	// Extract the SHA256 digest if available
	sha256 := ""
	if digest, ok := subject["digest"].(map[string]any); ok {
		if sha256Value, ok := digest["sha256"].(string); ok && sha256Value != "" {
			sha256 = sha256Value
		}
	}

	return name, sha256, nil
}
