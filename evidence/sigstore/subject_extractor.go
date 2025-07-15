package sigstore

import (
	"encoding/json"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

func ExtractSubjectFromBundle(b *bundle.Bundle) (repoPath string, err error) {
	envelope, err := GetDSSEEnvelope(b)
	if err != nil {
		return "", err
	}

	return extractSubjectFromEnvelope(envelope)
}

func extractSubjectFromEnvelope(envelope *protodsse.Envelope) (repoPath string, err error) {
	if envelope == nil {
		return "", errorutils.CheckErrorf("envelope is nil")
	}

	var statement map[string]interface{}
	if err := json.Unmarshal(envelope.Payload, &statement); err != nil {
		return "", errorutils.CheckErrorf("failed to parse statement from DSSE payload: %s", err.Error())
	}

	repoPath = extractRepoPathFromStatement(statement)

	return repoPath, nil
}

func extractRepoPathFromStatement(statement map[string]interface{}) string {
	if subjects, ok := statement["subject"].([]interface{}); ok && len(subjects) > 0 {
		if subject, ok := subjects[0].(map[string]interface{}); ok {
			if name, ok := subject["name"].(string); ok && name != "" {
				return name
			}
		}
	}

	if predicate, ok := statement["predicate"].(map[string]interface{}); ok {
		if artifact, ok := predicate["artifact"].(map[string]interface{}); ok {
			if path, ok := artifact["path"].(string); ok && path != "" {
				return path
			}
			if uri, ok := artifact["uri"].(string); ok && uri != "" {
				return uri
			}
		}

		if subject, ok := predicate["subject"].(map[string]interface{}); ok {
			if path, ok := subject["path"].(string); ok && path != "" {
				return path
			}
			if uri, ok := subject["uri"].(string); ok && uri != "" {
				return uri
			}
		}

		if materials, ok := predicate["materials"].([]interface{}); ok && len(materials) > 0 {
			if material, ok := materials[0].(map[string]interface{}); ok {
				if uri, ok := material["uri"].(string); ok && uri != "" {
					return uri
				}
			}
		}
	}

	return ""
}
