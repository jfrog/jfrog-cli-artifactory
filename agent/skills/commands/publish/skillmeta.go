package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// versionSafeRegex permits semver-like strings: digits, dots, hyphens, plus, and alphanumerics.
// It rejects path separators, "..", null bytes, and other characters that could cause path traversal.
var versionSafeRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-+]*$`)

type SkillMeta struct {
	Name        string
	Description string
	Version     string
}

// ParseSkillMeta reads a SKILL.md file and extracts YAML frontmatter metadata.
func ParseSkillMeta(skillDir string) (*SkillMeta, error) {
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	// #nosec G304 -- path is constructed from user-provided skill directory argument
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md at %s: %w", skillMDPath, err)
	}

	meta, err := parseFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse SKILL.md frontmatter: %w", err)
	}

	if meta.Name == "" {
		return nil, fmt.Errorf("SKILL.md missing required 'name' field in frontmatter")
	}

	return meta, nil
}

func parseFrontmatter(content string) (*SkillMeta, error) {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return nil, fmt.Errorf("SKILL.md does not start with YAML frontmatter delimiter '---'")
	}

	trimmed := strings.TrimSpace(content)
	// Find second --- delimiter
	rest := trimmed[3:]
	endIdx := strings.Index(rest, "---")
	if endIdx < 0 {
		return nil, fmt.Errorf("SKILL.md missing closing YAML frontmatter delimiter '---'")
	}

	frontmatter := rest[:endIdx]
	meta := &SkillMeta{}

	lines := strings.Split(frontmatter, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		value := stripQuotes(strings.TrimSpace(line[colonIdx+1:]))

		// Handle YAML block scalar indicators (> >- | |-): collect indented continuation lines.
		if value == ">" || value == ">-" || value == "|" || value == "|-" {
			var parts []string
			for i+1 < len(lines) {
				next := lines[i+1]
				if next != "" && !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
					break
				}
				i++
				parts = append(parts, strings.TrimSpace(next))
			}
			value = strings.TrimSpace(strings.Join(parts, " "))
		}

		switch key {
		case "name":
			meta.Name = value
		case "description":
			meta.Description = value
		case "version":
			meta.Version = value
		}
	}

	return meta, nil
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// versionLineRegex matches a YAML version field (with optional quoting) inside front matter.
var versionLineRegex = regexp.MustCompile(`(?m)^(version:\s*).*$`)

// UpdateSkillMetaVersion replaces the version value in the SKILL.md YAML front matter.
// It only acts when a version: line already exists; it never inserts a new one.
func UpdateSkillMetaVersion(skillDir, newVersion string) error {
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	// #nosec G304 -- path is constructed from user-provided skill directory argument
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return fmt.Errorf("failed to read SKILL.md at %s: %w", skillMDPath, err)
	}

	content := string(data)
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return fmt.Errorf("SKILL.md does not start with YAML frontmatter delimiter '---'")
	}

	rest := trimmed[3:]
	endIdx := strings.Index(rest, "---")
	if endIdx < 0 {
		return fmt.Errorf("SKILL.md missing closing YAML frontmatter delimiter '---'")
	}

	// Locate the front matter boundaries within the original (untrimmed) content
	fmStart := strings.Index(content, "---")
	fmEnd := strings.Index(content[fmStart+3:], "---")
	if fmEnd < 0 {
		return fmt.Errorf("SKILL.md missing closing YAML frontmatter delimiter '---'")
	}
	fmEnd += fmStart + 3

	frontmatter := content[fmStart+3 : fmEnd]
	if !versionLineRegex.MatchString(frontmatter) {
		return nil
	}

	updatedFM := versionLineRegex.ReplaceAllString(frontmatter, "${1}"+newVersion)
	updated := content[:fmStart+3] + updatedFM + content[fmEnd:]

	// #nosec G306 G703 -- SKILL.md is a user-owned source file; path constructed from user-provided skill directory
	if err := os.WriteFile(skillMDPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write updated SKILL.md: %w", err)
	}
	return nil
}

// ValidateSlug checks that a skill slug matches the required pattern.
func ValidateSlug(slug string) error {
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("invalid skill slug '%s': must match pattern ^[a-z0-9][a-z0-9-]*$", slug)
	}
	return nil
}

// ValidateVersion checks that a version string is safe for use in file paths.
// It rejects path traversal sequences and characters that could escape the intended directory.
func ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version must not be empty")
	}
	if strings.Contains(version, "..") {
		return fmt.Errorf("invalid version '%s': must not contain '..'", version)
	}
	if !versionSafeRegex.MatchString(version) {
		return fmt.Errorf("invalid version '%s': must contain only alphanumeric characters, dots, hyphens, and plus signs", version)
	}
	return nil
}
