package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const predicateTypePublishAttestation = "https://jfrog.com/evidence/publish-attestation/v1"

type predicate struct {
	Skill       string `json:"skill"`
	Version     string `json:"version"`
	PublishedAt string `json:"publishedAt"`
}

// GeneratePredicateFile writes the canonical predicate.json to a temp directory.
func GeneratePredicateFile(dir, slug, version string) (string, error) {
	p := predicate{
		Skill:       slug,
		Version:     version,
		PublishedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}

	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("failed to marshal predicate: %w", err)
	}

	path := filepath.Join(dir, "predicate.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write predicate file: %w", err)
	}
	return path, nil
}

// GenerateMarkdownFile writes the canonical attestation.md to a temp directory.
func GenerateMarkdownFile(dir, slug, version string) (string, error) {
	publishedAt := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	md := fmt.Sprintf(`# Publish Attestation

| Field | Value |
|-------|-------|
| Skill | %s |
| Version | %s |
| Published at | %s |
`, slug, version, publishedAt)

	path := filepath.Join(dir, "attestation.md")
	if err := os.WriteFile(path, []byte(md), 0600); err != nil {
		return "", fmt.Errorf("failed to write attestation markdown: %w", err)
	}
	return path, nil
}
