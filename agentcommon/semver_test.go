package agentcommon

import "testing"

func TestLatestVersion(t *testing.T) {
	got, err := LatestVersion([]string{"0.1.0", "1.2.3", "1.10.0", "1.3.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.10.0" {
		t.Fatalf("expected 1.10.0, got %s", got)
	}
}

func TestLatestVersionSkipsInvalid(t *testing.T) {
	got, err := LatestVersion([]string{"not-semver", "1.0.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.0.0" {
		t.Fatalf("expected 1.0.0, got %s", got)
	}
}

func TestLatestVersionEmpty(t *testing.T) {
	if _, err := LatestVersion(nil); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestLatestVersionAllInvalid(t *testing.T) {
	if _, err := LatestVersion([]string{"foo", "bar"}); err == nil {
		t.Fatal("expected error when no valid semvers")
	}
}

func TestNextMinorVersion(t *testing.T) {
	got, err := NextMinorVersion("1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.3.0" {
		t.Fatalf("expected 1.3.0, got %s", got)
	}
}
