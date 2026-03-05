package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

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

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		value := stripQuotes(strings.TrimSpace(line[colonIdx+1:]))

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

// ValidateSlug checks that a skill slug matches the required pattern.
func ValidateSlug(slug string) error {
	if !slugRegex.MatchString(slug) {
		return fmt.Errorf("invalid skill slug '%s': must match pattern ^[a-z0-9][a-z0-9-]*$", slug)
	}
	return nil
}
