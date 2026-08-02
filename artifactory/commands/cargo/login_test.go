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
	// Isolate the global cargo config (parseCargoRegistries also reads $CARGO_HOME/config.toml)
	// so the test only sees the project-local config it writes.
	t.Setenv("CARGO_HOME", t.TempDir())

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

// TestParseCargoRegistriesMergesGlobalConfig guards the fix for `jf setup cargo`: registries live in
// the global $CARGO_HOME/config.toml, so parseCargoRegistries must merge it with the project-local
// config (project wins on conflict).
func TestParseCargoRegistriesMergesGlobalConfig(t *testing.T) {
	cargoHomeDir := t.TempDir()
	t.Setenv("CARGO_HOME", cargoHomeDir)
	// Global config (what `jf setup cargo` writes): jfrog + jfrog-local.
	globalCfg := `
[registries.jfrog]
index = "sparse+https://acme.jfrog.io/artifactory/api/cargo/remote/index/"
[registries.jfrog-local]
index = "sparse+https://acme.jfrog.io/artifactory/api/cargo/local/index/"
`
	if err := os.WriteFile(filepath.Join(cargoHomeDir, "config.toml"), []byte(globalCfg), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	// Project-local config overrides jfrog's URL and adds a project-only registry.
	projDir := t.TempDir()
	projCargo := filepath.Join(projDir, ".cargo")
	if err := os.MkdirAll(projCargo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projCfg := `
[registries.jfrog]
index = "sparse+https://acme.jfrog.io/artifactory/api/cargo/project-override/index/"
[registries.proj-only]
index = "sparse+https://acme.jfrog.io/artifactory/api/cargo/proj-only/index/"
`
	if err := os.WriteFile(filepath.Join(projCargo, "config.toml"), []byte(projCfg), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	got := parseCargoRegistries(projDir)
	// jfrog-local comes from global; proj-only from project; jfrog is the project override.
	if got["jfrog-local"] != "sparse+https://acme.jfrog.io/artifactory/api/cargo/local/index/" {
		t.Errorf("jfrog-local (global) = %q", got["jfrog-local"])
	}
	if got["proj-only"] != "sparse+https://acme.jfrog.io/artifactory/api/cargo/proj-only/index/" {
		t.Errorf("proj-only (project) = %q", got["proj-only"])
	}
	if got["jfrog"] != "sparse+https://acme.jfrog.io/artifactory/api/cargo/project-override/index/" {
		t.Errorf("jfrog should be the project override, got %q", got["jfrog"])
	}
}

func TestCommandBucket(t *testing.T) {
	// install, build, and publish collect build-info. build shares the "deps" bucket with
	// install because both resolve + compile the same dep set; publish additionally records
	// the uploaded .crate artifact. Every other cargo command is a pass-through.
	cases := map[string]string{
		"install": "deps",
		"build":   "deps",
		"publish": "publish",
		// pass-through — collect nothing:
		"package": "none", "update": "none", "add": "none", "fetch": "none",
		"generate-lockfile": "none", "run": "none", "test": "none", "check": "none",
		"metadata": "none", "tree": "none", "search": "none", "--version": "none",
	}
	for cmd, want := range cases {
		if got := commandBucket(cmd); got != want {
			t.Errorf("commandBucket(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestNeedsRemoteAccess(t *testing.T) {
	// Only the collecting commands get auth injection; the rest are pass-throughs. build joined
	// install/publish once build-info collection was added for it — cargo build resolves crates
	// (needs the authenticated registry) before compiling.
	for _, cmd := range []string{"install", "build", "publish"} {
		if !needsRemoteAccess(cmd) {
			t.Errorf("needsRemoteAccess(%q) = false, want true", cmd)
		}
	}
	for _, cmd := range []string{"package", "fetch", "update", "add", "check", "test", "run", "metadata"} {
		if needsRemoteAccess(cmd) {
			t.Errorf("needsRemoteAccess(%q) = true, want false (pass-through)", cmd)
		}
	}
}

func TestCargoRegistryEnvKey(t *testing.T) {
	if got := cargoRegistryEnvKey("my-crates"); got != "CARGO_REGISTRIES_MY_CRATES_TOKEN" {
		t.Errorf("got %q", got)
	}
}

func TestBuildAuthEnv(t *testing.T) {
	// buildAuthEnv forwards the credential value verbatim (Bearer or Basic).
	got := buildAuthEnv("my-crates", "Bearer abc")
	want := []string{`CARGO_REGISTRIES_MY_CRATES_TOKEN=Bearer abc`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	gotBasic := buildAuthEnv("my-crates", "Basic dTpw")
	wantBasic := []string{`CARGO_REGISTRIES_MY_CRATES_TOKEN=Basic dTpw`}
	if !reflect.DeepEqual(gotBasic, wantBasic) {
		t.Errorf("got %v, want %v", gotBasic, wantBasic)
	}
	if len(buildAuthEnv("", "abc")) != 0 || len(buildAuthEnv("my-crates", "")) != 0 {
		t.Error("expected empty env when registry or credential missing")
	}
}
