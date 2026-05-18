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
	Plugin      string `json:"plugin"`
	Version     string `json:"version"`
	PublishedAt string `json:"publishedAt"`
}

func formatPublishedAt(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
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
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write predicate file: %w", err)
	}
	return path, nil
}

// GenerateMarkdownFile writes the canonical attestation.md to a temp directory.
func GenerateMarkdownFile(dir, slug, version string, publishedAt time.Time) (string, error) {
	md := fmt.Sprintf(`# Publish Attestation

| Field | Value |
|-------|-------|
| Plugin | %s |
| Version | %s |
| Published at | %s |
`, slug, version, formatPublishedAt(publishedAt))
	path := filepath.Join(dir, "attestation.md")
	if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
		return "", fmt.Errorf("failed to write attestation markdown: %w", err)
	}
	return path, nil
}
