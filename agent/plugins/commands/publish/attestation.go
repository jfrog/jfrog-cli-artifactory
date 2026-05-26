package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common"
)

const (
	predicateTypePublishAttestation = "https://jfrog.com/evidence/publish-attestation/v1"

	publishAttestationMarkdownTemplate = `# Publish Attestation

| Field | Value |
|-------|-------|
| Plugin | %s |
| Version | %s |
| Published at | %s |
`
)

type predicate struct {
	Plugin      string `json:"plugin"`
	Version     string `json:"version"`
	PublishedAt string `json:"publishedAt"`
}

func formatPublishedAt(publishedAt time.Time) string {
	return publishedAt.UTC().Format("2006-01-02T15:04:05Z")
}

// GeneratePredicateFile writes the canonical predicate.json to a temp directory.
func GeneratePredicateFile(dir, slug, version string, publishedAt time.Time) (string, error) {
	data, err := json.Marshal(predicate{
		Plugin:      slug,
		Version:     version,
		PublishedAt: formatPublishedAt(publishedAt),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal predicate: %w", err)
	}
	path := filepath.Join(dir, "predicate.json")
	if err := os.WriteFile(path, data, common.PrivateFileMode); err != nil {
		return "", fmt.Errorf("failed to write predicate file: %w", err)
	}
	return path, nil
}

func formatPublishAttestationMarkdown(slug, version string, publishedAt time.Time) string {
	return fmt.Sprintf(
		publishAttestationMarkdownTemplate,
		slug,
		version,
		formatPublishedAt(publishedAt),
	)
}

// GenerateMarkdownFile writes the canonical attestation.md to a temp directory.
func GenerateMarkdownFile(dir, slug, version string, publishedAt time.Time) (string, error) {
	path := filepath.Join(dir, "attestation.md")
	if err := os.WriteFile(path, []byte(formatPublishAttestationMarkdown(slug, version, publishedAt)), common.PrivateFileMode); err != nil {
		return "", fmt.Errorf("failed to write attestation markdown: %w", err)
	}
	return path, nil
}
