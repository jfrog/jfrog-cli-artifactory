package cargo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRegistryHostMatches(t *testing.T) {
	cases := []struct {
		indexURL       string
		artifactoryURL string
		want           bool
	}{
		{
			indexURL:       "sparse+https://acme.jfrog.io/artifactory/api/cargo/crates/",
			artifactoryURL: "https://acme.jfrog.io/artifactory/",
			want:           true,
		},
		{
			indexURL:       "https://acme.jfrog.io/artifactory/git/crates.git",
			artifactoryURL: "https://acme.jfrog.io/artifactory",
			want:           true,
		},
		{
			indexURL:       "sparse+https://crates.io/",
			artifactoryURL: "https://acme.jfrog.io/artifactory/",
			want:           false,
		},
		{
			indexURL:       "",
			artifactoryURL: "https://acme.jfrog.io/artifactory/",
			want:           false,
		},
		{
			indexURL:       "sparse+https://acme.jfrog.io/x",
			artifactoryURL: "",
			want:           false,
		},
	}
	for _, tc := range cases {
		got := registryHostMatches(tc.indexURL, tc.artifactoryURL)
		if got != tc.want {
			t.Errorf("registryHostMatches(%q, %q) = %v, want %v",
				tc.indexURL, tc.artifactoryURL, got, tc.want)
		}
	}
}

func TestParseCargoRegistries(t *testing.T) {
	// Empty dir — no config file — should return empty map without panic.
	empty := parseCargoRegistries(t.TempDir())
	if len(empty) != 0 {
		t.Errorf("expected empty map for missing config, got %v", empty)
	}

	// Dir with a valid config containing two registries.
	dir := t.TempDir()
	cargoDir := filepath.Join(dir, ".cargo")
	if err := os.MkdirAll(cargoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configContent := `
[registries.crates]
index = "sparse+https://acme.jfrog.io/artifactory/api/cargo/crates/"
[registries.internal]
index = "sparse+https://acme.jfrog.io/artifactory/api/cargo/internal/"
`
	if err := os.WriteFile(filepath.Join(cargoDir, "config.toml"), []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got := parseCargoRegistries(dir)
	if len(got) != 2 {
		t.Fatalf("expected 2 registries, got %d: %v", len(got), got)
	}
	wantURLs := map[string]string{
		"crates":   "sparse+https://acme.jfrog.io/artifactory/api/cargo/crates/",
		"internal": "sparse+https://acme.jfrog.io/artifactory/api/cargo/internal/",
	}
	for name, wantURL := range wantURLs {
		if got[name] != wantURL {
			t.Errorf("registry %q: got %q, want %q", name, got[name], wantURL)
		}
	}
}

func TestCommandBucket(t *testing.T) {
	cases := map[string]string{
		// "build" and "package" are intentionally not collected (bucket "none"); only "publish"
		// records artifacts.
		"build": "none", "package": "none", "install": "deps", "update": "deps", "add": "deps", "fetch": "deps",
		"publish": "publish",
		"metadata": "none", "tree": "none", "search": "none", "--version": "none",
	}
	for cmd, want := range cases {
		if got := commandBucket(cmd); got != want {
			t.Errorf("commandBucket(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestCargoRegistryEnvKey(t *testing.T) {
	if got := cargoRegistryEnvKey("my-crates"); got != "CARGO_REGISTRIES_MY_CRATES_TOKEN" {
		t.Errorf("got %q", got)
	}
}

func TestBuildAuthEnv(t *testing.T) {
	got := buildAuthEnv("my-crates", "abc")
	// "Bearer " scheme — required by Artifactory's Cargo index (verified live).
	want := []string{`CARGO_REGISTRIES_MY_CRATES_TOKEN=Bearer abc`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(buildAuthEnv("", "abc")) != 0 || len(buildAuthEnv("my-crates", "")) != 0 {
		t.Error("expected empty env when registry or token missing")
	}
}
