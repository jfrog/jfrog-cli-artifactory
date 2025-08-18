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
		return "", "", errorutils.CheckErrorf("bundle cannot be nil")
	}

	envelope, err := GetDSSEEnvelope(b)
	if err != nil {
		return "", "", fmt.Errorf("failed to get DSSE envelope: %w", err)
	}

	return extractSubjectFromEnvelope(envelope)
}

// extractSubjectFromEnvelope extracts the repository path and SHA256 checksum from a DSSE envelope
func extractSubjectFromEnvelope(envelope *protodsse.Envelope) (repoPath, sha256 string, err error) {
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

	repoPath, sha256 = extractRepoPathFromStatement(statement)
	return repoPath, sha256, nil
}

// extractRepoPathFromStatement extracts the repository path and SHA256 checksum from a statement
// The statement should contain a "subject" array with at least one subject object
// Each subject should have a "name" field and optionally a "digest" field with SHA256
func extractRepoPathFromStatement(statement map[string]any) (string, string) {
	if statement == nil {
		return "", ""
	}

	subjects, ok := statement["subject"].([]any)
	if !ok || len(subjects) == 0 {
		return "", ""
	}

	// Get the first subject
	subject, ok := subjects[0].(map[string]any)
	if !ok {
		return "", ""
	}

	// Extract the name
	name, ok := subject["name"].(string)
	if !ok || name == "" {
		return "", ""
	}

	// Extract the SHA256 digest if available
	sha256 := ""
	if digest, ok := subject["digest"].(map[string]any); ok {
		if sha256Value, ok := digest["sha256"].(string); ok && sha256Value != "" {
			sha256 = sha256Value
		}
	}

	return name, sha256
}
