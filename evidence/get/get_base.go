package get

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const SchemaVersion = "1.0"

type SubjectType string // Types in GetEvidence output

const (
	ArtifactType      SubjectType = "artifact"
	BuildType         SubjectType = "build"
	ReleaseBundleType SubjectType = "release-bundle"
)

type getEvidenceBase struct {
	serverDetails    *config.ServerDetails
	outputFileName   string
	format           string
	includePredicate bool
}

type JsonlLine struct {
	SchemaVersion string      `json:"schemaVersion"`
	Type          SubjectType `json:"type"`
	Result        any         `json:"result"`
}

type EvidenceEntry struct {
	PredicateSlug string         `json:"predicateSlug"`
	PredicateType string         `json:"predicateType,omitempty"`
	DownloadPath  string         `json:"downloadPath"`
	Verified      bool           `json:"verified"`
	SigningKey    map[string]any `json:"signingKey"`
	Subject       map[string]any `json:"subject"`
	CreatedBy     string         `json:"createdBy"`
	CreatedAt     string         `json:"createdAt"`
	Predicate     map[string]any `json:"predicate,omitempty"`
}

type CustomEvidenceResult struct {
	RepoPath string          `json:"repoPath"`
	Evidence []EvidenceEntry `json:"evidence"`
}

type ArtifactEvidence struct {
	Evidence    EvidenceEntry `json:"evidence"`
	PackageType string        `json:"package-type"`
	RepoPath    string        `json:"repo-path"`
}

type BuildEvidence struct {
	Evidence    EvidenceEntry `json:"evidence"`
	BuildName   string        `json:"build-name"`
	BuildNumber string        `json:"build-number"`
	StartedAt   string        `json:"started-at"`
}

type ReleaseBundleResult struct {
	ReleaseBundle        string             `json:"release-bundle"`
	ReleaseBundleVersion string             `json:"release-bundle-version"`
	Evidence             []EvidenceEntry    `json:"evidence"`
	Artifacts            []ArtifactEvidence `json:"artifacts,omitempty"`
	Builds               []BuildEvidence    `json:"builds,omitempty"`
}

func (g *getEvidenceBase) exportEvidenceToFile(evidence []byte, outputFileName, format string) error {
	if format == "" {
		format = "json"
	}

	switch format {
	case "json":
		return exportEvidenceToJsonFile(evidence, outputFileName)
	case "jsonl":
		return exportEvidenceToJsonlFile(evidence, outputFileName)
	default:
		log.Error("Unsupported format. Supported formats are: json, jsonl")
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func exportEvidenceToJsonFile(evidence []byte, outputFileName string) error {
	if outputFileName == "" {
		// Stream to console
		fmt.Println(string(evidence))
		return nil
	}

	file, err := os.Create(outputFileName)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = file.Write(evidence)
	if err != nil {
		return err
	}

	log.Info("Evidence successfully exported to file name: ", outputFileName)
	return nil
}

func exportEvidenceToJsonlFile(data []byte, outputFileName string) error {
	if outputFileName == "" {
		return writeEvidenceJsonl(data, os.Stdout)
	}

	file, err := os.Create(outputFileName)
	if err != nil {
		return err
	}
	defer file.Close()

	return writeEvidenceJsonl(data, file)
}

// writeEvidenceJsonl handles evidence output structures that have schemaVersion, type, and result fields
func writeEvidenceJsonl(data []byte, file *os.File) error {
	var evidenceOutput map[string]any
	if err := json.Unmarshal(data, &evidenceOutput); err != nil {
		return fmt.Errorf("failed to parse evidence output: %w", err)
	}

	schemaVersion, _ := evidenceOutput["schemaVersion"].(string)
	typeString, _ := evidenceOutput["type"].(string)

	typeField := SubjectType(typeString)

	log.Debug("Processing evidence with type:", typeField)

	if typeField == ReleaseBundleType {
		var releaseBundleOutput ReleaseBundleOutput
		if err := json.Unmarshal(data, &releaseBundleOutput); err != nil {
			return fmt.Errorf("failed to parse release bundle output: %w", err)
		}
		return writeReleaseBundleJsonlFromStruct(schemaVersion, typeField, releaseBundleOutput.Result, file)
	} else {
		var customEvidenceOutput CustomEvidenceOutput
		if err := json.Unmarshal(data, &customEvidenceOutput); err != nil {
			return fmt.Errorf("failed to parse custom evidence output: %w", err)
		}
		return writeCustomEvidenceJsonl(schemaVersion, typeField, customEvidenceOutput.Result, file)
	}
}

func writeCustomEvidenceJsonl(schemaVersion string, typeField SubjectType, result CustomEvidenceResult, file *os.File) error {
	// Write each evidence entry as a separate line
	for _, evidence := range result.Evidence {
		lineWithMetadata := JsonlLine{
			SchemaVersion: schemaVersion,
			Type:          typeField,
			Result:        evidence,
		}
		jsonLine, err := json.Marshal(lineWithMetadata)
		if err != nil {
			return fmt.Errorf("failed to marshal custom evidence line: %w", err)
		}
		if _, err := file.Write(append(jsonLine, '\n')); err != nil {
			return fmt.Errorf("failed to write evidence line: %w", err)
		}
	}

	if file != os.Stdout {
		log.Info("Evidence successfully exported to file name: ", file.Name())
	}

	return nil
}

func writeReleaseBundleJsonlFromStruct(schemaVersion string, typeField SubjectType, result ReleaseBundleResult, file *os.File) error {
	for _, evidence := range result.Evidence {
		lineWithMetadata := JsonlLine{
			SchemaVersion: schemaVersion,
			Type:          typeField,
			Result:        evidence,
		}
		jsonLine, err := json.Marshal(lineWithMetadata)
		if err != nil {
			return fmt.Errorf("failed to marshal release bundle evidence line: %w", err)
		}
		if _, err := file.Write(append(jsonLine, '\n')); err != nil {
			return fmt.Errorf("failed to write evidence line: %w", err)
		}
	}

	for _, artifact := range result.Artifacts {
		lineWithMetadata := JsonlLine{
			SchemaVersion: schemaVersion,
			Type:          ArtifactType,
			Result:        artifact,
		}
		jsonLine, err := json.Marshal(lineWithMetadata)
		if err != nil {
			return fmt.Errorf("failed to marshal artifact evidence line: %w", err)
		}
		if _, err := file.Write(append(jsonLine, '\n')); err != nil {
			return fmt.Errorf("failed to write evidence line: %w", err)
		}
	}

	for _, build := range result.Builds {
		lineWithMetadata := JsonlLine{
			SchemaVersion: schemaVersion,
			Type:          BuildType,
			Result:        build,
		}
		jsonLine, err := json.Marshal(lineWithMetadata)
		if err != nil {
			return fmt.Errorf("failed to marshal build evidence line: %w", err)
		}
		if _, err := file.Write(append(jsonLine, '\n')); err != nil {
			return fmt.Errorf("failed to write evidence line: %w", err)
		}
	}

	if file != os.Stdout {
		log.Info("Evidence successfully exported to file name: ", file.Name())
	}

	return nil
}

func createOrderedEvidenceEntry(node map[string]any, includePredicate bool) EvidenceEntry {
	entry := EvidenceEntry{}

	if predicateSlug, ok := node["predicateSlug"].(string); ok {
		entry.PredicateSlug = predicateSlug
	}

	if predicateType, ok := node["predicateType"].(string); ok && predicateType != "" {
		entry.PredicateType = predicateType
	}

	if downloadPath, ok := node["downloadPath"].(string); ok {
		entry.DownloadPath = downloadPath
	}

	if verified, ok := node["verified"].(bool); ok {
		entry.Verified = verified
	}

	if signingKey, ok := node["signingKey"].(map[string]any); ok {
		entry.SigningKey = signingKey
	}

	if subject, ok := node["subject"].(map[string]any); ok {
		entry.Subject = subject
	}

	if createdBy, ok := node["createdBy"].(string); ok {
		entry.CreatedBy = createdBy
	}

	if createdAt, ok := node["createdAt"].(string); ok {
		entry.CreatedAt = createdAt
	}

	if includePredicate {
		if predicate, ok := node["predicate"].(map[string]any); ok {
			entry.Predicate = predicate
		}
	}

	return entry
}
