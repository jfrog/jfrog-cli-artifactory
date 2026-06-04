package common

import "testing"

func TestValidateSlug(t *testing.T) {
	good := []string{"foo", "foo-bar", "foo123", "a", "4chan-reader", "my-skill", "skill123"}
	for _, slug := range good {
		if err := ValidateSlug(slug); err != nil {
			t.Fatalf("slug %q should be valid: %v", slug, err)
		}
	}
	bad := []string{"", "-foo", "Foo", "foo bar", "foo/bar", "My-Skill", "-invalid", "has space"}
	for _, slug := range bad {
		if err := ValidateSlug(slug); err == nil {
			t.Fatalf("slug %q should be invalid", slug)
		}
	}
}
