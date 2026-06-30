package cargo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCrateRepoPath(t *testing.T) {
	path, name, version := crateRepoPath("serde-1.0.197.crate")
	if path != "serde/1.0.197/serde-1.0.197.crate" || name != "serde" || version != "1.0.197" {
		t.Errorf("got (%q,%q,%q)", path, name, version)
	}
	// hyphenated crate name
	path, name, version = crateRepoPath("my-crate-0.2.0.crate")
	if path != "my-crate/0.2.0/my-crate-0.2.0.crate" || name != "my-crate" || version != "0.2.0" {
		t.Errorf("hyphenated: got (%q,%q,%q)", path, name, version)
	}
}

func TestScanCrateArtifacts(t *testing.T) {
	wd := t.TempDir()
	pkgDir := filepath.Join(wd, "target", "package")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "serde-1.0.197.crate"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	arts, err := scanCrateArtifacts(wd, "cargo-local")
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	a := arts[0]
	if a.Name != "serde-1.0.197.crate" || a.Type != "crate" || a.Path != "serde/1.0.197/serde-1.0.197.crate" || a.OriginalDeploymentRepo != "cargo-local" {
		t.Errorf("artifact fields wrong: %+v", a)
	}
	// entities.Artifact embeds entities.Checksum, so Sha256 is promoted
	if a.Sha256 == "" {
		t.Error("expected local checksum computed")
	}
}
