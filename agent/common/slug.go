package common

import (
	"fmt"
	"unicode"
)

// ValidateSlug checks that a package slug is safe for repository paths and Artifactory layout.
// It must start with a lowercase letter or digit, then contain only lowercase letters, digits, or hyphens.
//
// Accepted examples: "my-plugin", "skill123", "a", "4chan-reader"
// Not accepted examples: "", "-invalid", "My-Skill", "has space", "foo/bar"
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("invalid slug %q: must not be empty", slug)
	}
	if !isSlugStartChar(slug[0]) {
		return fmt.Errorf("invalid slug %q: must start with a lowercase letter or digit", slug)
	}
	for charIndex := 1; charIndex < len(slug); charIndex++ {
		c := slug[charIndex]
		if c != '-' && !isSlugStartChar(c) {
			return fmt.Errorf("invalid slug %q: may contain only lowercase letters, digits, and hyphens", slug)
		}
	}
	return nil
}

func isSlugStartChar(character byte) bool {
	r := rune(character)
	return unicode.IsLower(r) || unicode.IsDigit(r)
}
